// Package storage implements the storage sub-commands.
package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/oasisprotocol/oasis-core/go/common/crypto/hash"
	"github.com/oasisprotocol/oasis-core/go/common/logging"
	cmdCommon "github.com/oasisprotocol/oasis-core/go/oasis-node/cmd/common"
	roothash "github.com/oasisprotocol/oasis-core/go/roothash/api"
	"github.com/oasisprotocol/oasis-core/go/runtime/history"
	"github.com/oasisprotocol/oasis-core/go/runtime/registry"
	db "github.com/oasisprotocol/oasis-core/go/storage/mkvs/db/api"
	"github.com/oasisprotocol/oasis-core/go/storage/mkvs/db/badger"
	"github.com/oasisprotocol/oasis-core/go/storage/mkvs/node"
	workerStorage "github.com/oasisprotocol/oasis-core/go/worker/storage"
)

var (
	storageCmd = &cobra.Command{
		Use:   "storage",
		Short: "storage node utilities",
	}

	storageMigrateCmd = &cobra.Command{
		Use:   "migrate",
		Short: "perform node database migration",
		Run:   doMigrate,
	}

	storageCheckCmd = &cobra.Command{
		Use:   "check",
		Short: "check node databases for consistency",
		Run:   doCheck,
	}

	logger = logging.GetLogger("cmd/storage")

	pretty = cmdCommon.Isatty(1)
)

type displayHelper struct {
	lastTime     time.Time
	lastStatus   string
	lastProgress bool
}

func (dh *displayHelper) display(base, format string, args ...interface{}) {
	dh.lastTime = time.Time{}
	if pretty {
		if dh.lastProgress {
			fmt.Printf("\n"+format, args...)
		} else {
			fmt.Printf(format, args...)
		}
	} else {
		logger.Info(base)
	}
	dh.lastProgress = false
	dh.lastStatus = base
}

func (dh *displayHelper) DisplayStepBegin(msg string) {
	dh.display(msg, "- %s... ", msg)
}

func (dh *displayHelper) DisplayStepEnd(msg string) {
	dh.display(msg, "\r- %s: %s\n", dh.lastStatus, msg)
}

func (dh *displayHelper) DisplayStep(msg string) {
	dh.display(msg, "- %s...\n", msg)
}

func (dh *displayHelper) DisplayProgress(msg string, current, total uint64) {
	if pretty {
		if time.Since(dh.lastTime).Seconds() < 0.1 {
			return
		}
		dh.lastTime = time.Now()

		var leadin string
		if len(dh.lastStatus) > 0 {
			leadin = fmt.Sprintf("- %s:", dh.lastStatus)
		} else {
			leadin = "-"
		}
		fmt.Printf("\r%s %s %.2f%% (%d / %d)\033[K", leadin, msg, (float64(current)/float64(total))*100.0, current, total)
		dh.lastProgress = true
	}
}

type migrateHelper struct {
	displayHelper

	ctx     context.Context
	history history.History
	roots   map[hash.Hash]node.RootType
}

func (mh *migrateHelper) GetRootForHash(root hash.Hash, version uint64) (*node.Root, error) {
	block, err := mh.history.GetBlock(mh.ctx, version)
	if err != nil {
		if errors.Is(err, roothash.ErrNotFound) {
			return nil, badger.ErrVersionNotFound
		}
		return nil, err
	}

	for _, blockRoot := range block.Header.StorageRoots() {
		if blockRoot.Hash.Equal(&root) {
			return &blockRoot, nil
		}
	}
	return nil, nil
}

func doMigrate(cmd *cobra.Command, args []string) {
	dataDir := cmdCommon.DataDir()
	ctx := context.Background()

	runtimes, err := registry.ParseRuntimeMap(viper.GetStringSlice(registry.CfgSupported))
	if err != nil {
		logger.Error("unable to enumerate configured runtimes", "err", err)
		return
	}

	for rt := range runtimes {
		if pretty {
			fmt.Printf(" ** Upgrading storage database for runtime %v...\n", rt)
		}
		err := func() error {
			runtimeDir := registry.GetRuntimeStateDir(dataDir, rt)

			history, err := history.New(runtimeDir, rt, nil)
			if err != nil {
				return fmt.Errorf("error creating history provider: %w", err)
			}
			defer history.Close()

			nodeCfg := &db.Config{
				DB:        workerStorage.GetLocalBackendDBDir(runtimeDir, viper.GetString(workerStorage.CfgBackend)),
				Namespace: rt,
			}

			helper := &migrateHelper{
				ctx:     ctx,
				history: history,
				roots:   map[hash.Hash]node.RootType{},
			}

			newVersion, err := badger.Migrate(nodeCfg, helper)
			if err != nil {
				return fmt.Errorf("node database migrator returned error: %w", err)
			}
			logger.Info("successfully migrated node database", "new_version", newVersion)
			return nil
		}()
		if err != nil {
			logger.Error("error upgrading runtime", "rt", rt, "err", err)
			if pretty {
				fmt.Printf("error upgrading runtime %v: %v\n", rt, err)
			}
			return
		}
	}
}

func doCheck(cmd *cobra.Command, args []string) {
	dataDir := cmdCommon.DataDir()
	ctx := context.Background()

	runtimes, err := registry.ParseRuntimeMap(viper.GetStringSlice(registry.CfgSupported))
	if err != nil {
		logger.Error("unable to enumerate configured runtimes", "err", err)
		return
	}

	for rt := range runtimes {
		if pretty {
			fmt.Printf("Checking storage database for runtime %v...\n", rt)
		}
		err := func() error {
			runtimeDir := registry.GetRuntimeStateDir(dataDir, rt)

			nodeCfg := &db.Config{
				DB:        workerStorage.GetLocalBackendDBDir(runtimeDir, viper.GetString(workerStorage.CfgBackend)),
				Namespace: rt,
			}

			display := &displayHelper{}

			err := badger.CheckSanity(ctx, nodeCfg, display)
			if err != nil {
				return fmt.Errorf("node database checker returned error: %w", err)
			}
			logger.Info("node database seems to be error-free", "rt", rt)
			return nil
		}()
		if err != nil {
			logger.Error("error checking node database", "rt", rt, "err", err)
			if pretty {
				fmt.Printf("error checking node database for runtime %v: %v\n", rt, err)
			}
			return
		}
	}
}

// Register registers the client sub-command and all of its children.
func Register(parentCmd *cobra.Command) {
	storageMigrateCmd.Flags().AddFlagSet(registry.Flags)
	storageCheckCmd.Flags().AddFlagSet(registry.Flags)
	storageCmd.AddCommand(storageMigrateCmd)
	storageCmd.AddCommand(storageCheckCmd)
	parentCmd.AddCommand(storageCmd)
}
