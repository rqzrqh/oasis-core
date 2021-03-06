package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common/cbor"
	"github.com/oasisprotocol/oasis-core/go/oasis-test-runner/env"
	"github.com/oasisprotocol/oasis-core/go/oasis-test-runner/oasis"
	"github.com/oasisprotocol/oasis-core/go/oasis-test-runner/scenario"
	"github.com/oasisprotocol/oasis-core/go/runtime/client/api"
	runtimeClient "github.com/oasisprotocol/oasis-core/go/runtime/client/api"
	runtimeTransaction "github.com/oasisprotocol/oasis-core/go/runtime/transaction"
)

// LateStart is the LateStart node basic scenario.
var LateStart scenario.Scenario = newLateStartImpl("late-start", "simple-keyvalue-client", nil)

const lateStartInitialWait = 2 * time.Minute

type lateStartImpl struct {
	runtimeImpl
}

func newLateStartImpl(name, clientBinary string, clientArgs []string) scenario.Scenario {
	return &lateStartImpl{
		runtimeImpl: *newRuntimeImpl(name, clientBinary, clientArgs),
	}
}

func (sc *lateStartImpl) Clone() scenario.Scenario {
	return &lateStartImpl{
		runtimeImpl: *sc.runtimeImpl.Clone().(*runtimeImpl),
	}
}

func (sc *lateStartImpl) Fixture() (*oasis.NetworkFixture, error) {
	f, err := sc.runtimeImpl.Fixture()
	if err != nil {
		return nil, err
	}

	// Start without a client.
	f.Clients = []oasis.ClientFixture{}

	return f, nil
}

func (sc *lateStartImpl) Run(childEnv *env.Env) error {
	ctx := context.Background()

	// Start the network.
	var err error
	if err = sc.Net.Start(); err != nil {
		return err
	}

	sc.Logger.Info("Waiting before starting the client node",
		"wait_for", lateStartInitialWait,
	)
	time.Sleep(lateStartInitialWait)

	sc.Logger.Info("Starting the client node")
	clientFixture := &oasis.ClientFixture{}
	client, err := clientFixture.Create(sc.Net)
	if err != nil {
		return err
	}
	if err = client.Start(); err != nil {
		return err
	}

	ctrl, err := oasis.NewController(client.SocketPath())
	if err != nil {
		return fmt.Errorf("failed to create controller for client: %w", err)
	}
	_, err = ctrl.RuntimeClient.SubmitTx(ctx, &runtimeClient.SubmitTxRequest{
		RuntimeID: runtimeID,
		Data: cbor.Marshal(&runtimeTransaction.TxnCall{
			Method: "insert",
			Args: struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}{
				Key:   "hello",
				Value: "test",
			},
		}),
	})
	if !errors.Is(err, api.ErrNotSynced) {
		return fmt.Errorf("expected error: %v, got: %v", api.ErrNotSynced, err)
	}

	sc.Logger.Info("Starting the basic test client")
	cmd, err := sc.startClient(childEnv)
	if err != nil {
		return err
	}
	clientErrCh := make(chan error)
	go func() {
		clientErrCh <- cmd.Wait()
	}()

	return sc.wait(childEnv, cmd, clientErrCh)
}
