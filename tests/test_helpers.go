package tests

import (
	"fmt"
	"os"

	dbm "github.com/cosmos/cosmos-db"

	"cosmossdk.io/log/v2"
	pruningtypes "github.com/cosmos/cosmos-sdk/store/v2/pruning/types"

	bam "github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"

	"github.com/libertalia1724/praxis-core/app"
)

// NewTestNetworkFixture returns a new PraxisApp network fixture for e2e tests.
func NewTestNetworkFixture() network.TestFixture {
	dir, err := os.MkdirTemp("", "praxis")
	if err != nil {
		panic(fmt.Sprintf("failed creating temporary directory: %v", err))
	}
	defer func() { _ = os.RemoveAll(dir) }()

	newApp := app.NewPraxisApp(log.NewNopLogger(), dbm.NewMemDB(), true, simtestutil.NewAppOptionsWithFlagHome(dir))

	appCtr := func(val network.ValidatorI) servertypes.Application {
		return app.NewPraxisApp(
			val.GetCtx().Logger, dbm.NewMemDB(), true,
			simtestutil.NewAppOptionsWithFlagHome(val.GetCtx().Config.RootDir),
			bam.SetPruning(pruningtypes.NewPruningOptionsFromString(val.GetAppConfig().Pruning)),
			bam.SetMinGasPrices(val.GetAppConfig().MinGasPrices),
			bam.SetChainID(val.GetCtx().Viper.GetString(flags.FlagChainID)),
		)
	}

	return network.TestFixture{
		AppConstructor: appCtr,
		GenesisState:   newApp.DefaultGenesis(),
		EncodingConfig: testutil.TestEncodingConfig{
			InterfaceRegistry: newApp.InterfaceRegistry(),
			Codec:             newApp.AppCodec(),
			TxConfig:          newApp.TxConfig(),
			Amino:             newApp.LegacyAmino(),
		},
	}
}
