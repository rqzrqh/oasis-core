package roothash

import (
	"fmt"

	"github.com/oasisprotocol/oasis-core/go/common"
	"github.com/oasisprotocol/oasis-core/go/common/crypto/signature"
	"github.com/oasisprotocol/oasis-core/go/common/quantity"
	abciAPI "github.com/oasisprotocol/oasis-core/go/consensus/tendermint/api"
	registryState "github.com/oasisprotocol/oasis-core/go/consensus/tendermint/apps/registry/state"
	stakingState "github.com/oasisprotocol/oasis-core/go/consensus/tendermint/apps/staking/state"
	registry "github.com/oasisprotocol/oasis-core/go/registry/api"
	roothash "github.com/oasisprotocol/oasis-core/go/roothash/api"
	staking "github.com/oasisprotocol/oasis-core/go/staking/api"
)

func onEvidenceRuntimeEquivocation(
	ctx *abciAPI.Context,
	pk signature.PublicKey,
	runtimeID common.Namespace,
	penaltyAmount *quantity.Quantity,
) error {
	regState := registryState.NewMutableState(ctx.State())
	stakeState := stakingState.NewMutableState(ctx.State())

	node, err := regState.Node(ctx, pk)
	if err != nil {
		// Node might not exist anymore (old evidence). Or the submitted evidence
		// could be for a non-existing node (submitting "fake" but valid evidence).
		ctx.Logger().Error("failed to get runtime node by signature public key",
			"public_key", pk,
			"err", err,
		)
		return fmt.Errorf("tendermint/roothash: failed to get node by id %s: %w", pk, roothash.ErrInvalidEvidence)
	}

	// Slash runtime node entity
	entityAddr := staking.NewAddress(node.EntityID)
	totalSlashed, err := stakeState.SlashEscrow(ctx, entityAddr, penaltyAmount)
	if err != nil {
		return fmt.Errorf("tendermint/roothash: error slashing account %s: %w", entityAddr, err)
	}
	if totalSlashed.IsZero() {
		return fmt.Errorf("tendermint/roothash: nothing to slash from account %s", entityAddr)
	}

	// Move slashed amount to the runtime account.
	// TODO: part of slashed amount (configurable) should be transferred to the transaction submitter (should be escrowed?).
	runtimeAddr := staking.NewRuntimeAddress(runtimeID)
	if _, err := stakeState.TransferFromCommon(ctx, runtimeAddr, totalSlashed); err != nil {
		return fmt.Errorf("tendermint/roothash: failed transferring reward to runtime account %s: %w", runtimeAddr, err)
	}

	return nil
}

func onRuntimeIncorrectResults(
	ctx *abciAPI.Context,
	discrepancyCausers []signature.PublicKey,
	discrepancyResolvers []signature.PublicKey,
	runtimeID common.Namespace,
	penaltyAmount *quantity.Quantity,
) error {
	regState := registryState.NewMutableState(ctx.State())
	stakeState := stakingState.NewMutableState(ctx.State())

	var totalSlashed quantity.Quantity
	for _, pk := range discrepancyCausers {
		// Lookup the node entity.
		node, err := regState.Node(ctx, pk)
		if err == registry.ErrNoSuchNode {
			ctx.Logger().Error("runtime node not found by commitment signature public key",
				"public_key", pk,
			)
			continue
		}
		if err != nil {
			ctx.Logger().Error("failed to get runtime node by commitment signature public key",
				"public_key", pk,
				"err", err,
			)
			return fmt.Errorf("tendermint/roothash: getting node %s: %w", pk, err)
		}
		entityAddr := staking.NewAddress(node.EntityID)

		// Slash entity.
		slashed, err := stakeState.SlashEscrow(ctx, entityAddr, penaltyAmount)
		if err != nil {
			return fmt.Errorf("tendermint/roothash: error slashing account %s: %w", entityAddr, err)
		}
		if err = totalSlashed.Add(slashed); err != nil {
			return fmt.Errorf("tendermint/roothash: totalSlashed.Add(slashed): %w", err)
		}
		ctx.Logger().Debug("runtime node entity slashed for incorrect results",
			"slashed", slashed,
			"total_slashed", totalSlashed,
			"addr", entityAddr,
		)
	}

	// It can happen that nothing was slashed as nodes could be out of stake.
	// A node can be out of stake as stake claims are only checked on epoch transitions
	// and a node can be slashed multiple times per epoch.
	// This should not fail the round, as otherwise a single node without stake could
	// cause round failures until it is removed from the committee (on the next epoch transition).
	if totalSlashed.IsZero() {
		// Nothing more to do in this case.
		return nil
	}

	// Distribute slashed funds.
	// Runtime account reward.
	// TODO: make portion that is transferred to the runtime account configurable.
	runtimePercentage := 50
	runtimeAccReward := totalSlashed.Clone()
	if err := runtimeAccReward.Mul(quantity.NewFromUint64(uint64(runtimePercentage))); err != nil {
		return fmt.Errorf("tendermint/roothash: runtimeAccReward.Mul: %w", err)
	}
	if err := runtimeAccReward.Quo(quantity.NewFromUint64(uint64(100))); err != nil {
		return fmt.Errorf("tendermint/roothash: runtimeAccReward.Quo(100): %w", err)
	}
	runtimeAddr := staking.NewRuntimeAddress(runtimeID)
	if _, err := stakeState.TransferFromCommon(ctx, runtimeAddr, runtimeAccReward); err != nil {
		return fmt.Errorf("tendermint/roothash: failed transferring reward to %s: %w", runtimeAddr, err)
	}
	ctx.Logger().Debug("runtime account awarded slashed funds",
		"reward", runtimeAccReward,
		"total_slashed", totalSlashed,
		"runtime_addr", runtimeAddr,
	)

	// Backup workers reward.
	// TODO: should probably be escrowed.
	var rewardEntities []signature.PublicKey
	for _, pk := range discrepancyResolvers {
		node, err := regState.Node(ctx, pk)
		if err == registry.ErrNoSuchNode {
			ctx.Logger().Error("runtime node not found by commitment signature public key",
				"public_key", pk,
			)
			continue
		}
		if err != nil {
			ctx.Logger().Error("failed to get runtime node by commitment signature public key",
				"public_key", pk,
				"err", err,
			)
			return fmt.Errorf("tendermint/roothash: getting node %s: %w", pk, err)
		}
		rewardEntities = append(rewardEntities, node.EntityID)
	}
	if len(rewardEntities) == 0 {
		// Nothing more to do.
		ctx.Logger().Debug("no entities to reward")
		return nil
	}

	// (totalSlashed - runtimeAccReward) / n_reward_entities
	entityReward := totalSlashed.Clone()
	if err := entityReward.Sub(runtimeAccReward); err != nil {
		return fmt.Errorf("tendermint/roothash: remainingReward.Sub(runtimeAccReward): %w", err)
	}
	if err := entityReward.Quo(quantity.NewFromUint64(uint64(len(rewardEntities)))); err != nil {
		return fmt.Errorf("tendermint/roothash: remainingReward.Quo(len(discrepancyResolvers)): %w", err)
	}

	for _, pk := range rewardEntities {
		entAddr := staking.NewAddress(pk)
		// TODO: should be escrowed.
		if _, err := stakeState.TransferFromCommon(ctx, entAddr, entityReward); err != nil {
			return fmt.Errorf("tendermint/roothash: failed transferring reward to %s: %w", entAddr, err)
		}
		ctx.Logger().Debug("entity account awarded slashed funds",
			"reward", entityReward,
			"total_slashed", totalSlashed,
			"entity_addr", entAddr,
		)
	}

	return nil
}
