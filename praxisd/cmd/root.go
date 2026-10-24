package cmd

import (
	"os"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/libertalia1724/praxis-core/app"
	"github.com/spf13/cobra"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/config"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/server"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/cosmos/cosmos-sdk/x/auth/types"
)

// NewRootCmd creates a new root command for praxisd. It is called once in the
// main function.
func NewRootCmd() *cobra.Command {
	// we "pre"-instantiate the application for getting the injected/configured encoding configuration.
	//
	// NOTE: the wasm keeper creates its vm cache directory under the app home, so a throwaway
	// temp dir is used here (and removed afterwards) instead of the real node home. otherwise
	// every cli invocation would create wasm directories in ~/.praxisapp
	temp, err := os.MkdirTemp("", "praxisd")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	defer os.RemoveAll(temp)
	tempApp := app.NewPraxisApp(log.NewNopLogger(), dbm.NewMemDB(), false, simtestutil.NewAppOptionsWithFlagHome(temp))
	encodingConfig := app.EncodingConfig{
		InterfaceRegistry: tempApp.InterfaceRegistry(),
		Codec:             tempApp.AppCodec(),
		TxConfig:          tempApp.TxConfig(),
		Amino:             tempApp.LegacyAmino(),
	}

	initClientCtx := client.Context{}.
		WithCodec(encodingConfig.Codec).
		WithInterfaceRegistry(encodingConfig.InterfaceRegistry).
		WithTxConfig(encodingConfig.TxConfig).
		WithLegacyAmino(encodingConfig.Amino).
		WithInput(os.Stdin).
		WithAccountRetriever(types.AccountRetriever{}).
		WithHomeDir(app.DefaultNodeHome).
		WithViper("") // uses by default the binary name as prefix

	rootCmd := &cobra.Command{
		Use:           "praxisd",
		Short:         "praxis app",
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// set the default command outputs
			cmd.SetOut(cmd.OutOrStdout())
			cmd.SetErr(cmd.ErrOrStderr())

			initClientCtx = initClientCtx.WithCmdContext(cmd.Context())
			initClientCtx, err := client.ReadPersistentCommandFlags(initClientCtx, cmd.Flags())
			if err != nil {
				return err
			}

			initClientCtx, err = config.ReadFromClientConfig(initClientCtx)
			if err != nil {
				return err
			}

			if err := client.SetCmdClientContextHandler(initClientCtx, cmd); err != nil {
				return err
			}

			customAppTemplate, customAppConfig := initAppConfig()
			customCMTConfig := initCometBFTConfig()

			return server.InterceptConfigsPreRunHandler(cmd, customAppTemplate, customAppConfig, customCMTConfig)
		},
	}

	initRootCmd(rootCmd, encodingConfig.TxConfig, tempApp.BasicModuleManager)

	// add keyring to autocli opts
	autoCliOpts := tempApp.AutoCliOpts()
	autoCliOpts.ClientCtx = initClientCtx

	nodeCmds := nodeservice.NewNodeCommands()
	autoCliOpts.ModuleOptions[nodeCmds.Name()] = nodeCmds.AutoCLIOptions()

	if err := autoCliOpts.EnhanceRootCommand(rootCmd); err != nil {
		panic(err)
	}

	return rootCmd
}
