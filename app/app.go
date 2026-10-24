package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	"cosmossdk.io/client/v2/autocli"
	"cosmossdk.io/client/v2/helpers"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/log/v2"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	db "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	nodeservice "github.com/cosmos/cosmos-sdk/client/grpc/node"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	runtimeservices "github.com/cosmos/cosmos-sdk/runtime/services"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/std"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensus "github.com/cosmos/cosmos-sdk/x/consensus"
	consensusparamkeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	consensusparamtypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/cosmos-sdk/x/tx/signing"
	"github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cast"

	"github.com/CosmWasm/wasmd/x/wasm"
	wasmkeeper "github.com/CosmWasm/wasmd/x/wasm/keeper"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	// counter tutorial app wiring 1: add counter imports below
)

const (
	appName = "praxisApp"
)

var (
	DefaultNodeHome string

	_ runtime.AppI            = (*PraxisApp)(nil)
	_ servertypes.Application = (*PraxisApp)(nil)

	// module account permissions
	maccPerms = map[string][]string{
		authtypes.FeeCollectorName:          nil,
		distrtypes.ModuleName:               nil,
		stakingtypes.BondedPoolName:         {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName:      {authtypes.Burner, authtypes.Staking},
		stakingtypes.KeyRotationFeePoolName: {authtypes.Burner},
		govtypes.ModuleName:                 {authtypes.Burner},
		wasmtypes.ModuleName:                {authtypes.Burner},
	}
)

func init() {
	sdk.DefaultBondDenom = GetMicroPraxisDenom()
	SetConfig()
	var err error
	DefaultNodeHome, err = helpers.GetNodeHomeDirectory(".praxisapp")
	if err != nil {
		panic(err)
	}
}

type PraxisApp struct {
	*baseapp.BaseApp
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry

	keys map[string]*storetypes.KVStoreKey

	// Keepers
	AccountKeeper         authkeeper.AccountKeeper
	BankKeeper            bankkeeper.Keeper
	DistrKeeper           distrkeeper.Keeper
	GovKeeper             govkeeper.Keeper
	StakingKeeper         *stakingkeeper.Keeper
	SlashingKeeper        slashingkeeper.Keeper
	ConsensusParamsKeeper consensusparamkeeper.Keeper
	WasmKeeper            wasmkeeper.Keeper
	// counter tutorial app wiring 2: add the counter keeper field below

	// the module manager
	ModuleManager      *module.Manager
	BasicModuleManager module.BasicManager

	// simulation manager
	sm *module.SimulationManager

	// module configurator
	configurator module.Configurator
}

func NewPraxisApp(
	logger log.Logger,
	db db.DB,
	loadLatest bool,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *PraxisApp {
	interfaceRegistry, _ := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: proto.HybridResolver,
		SigningOptions: signing.Options{
			AddressCodec: address.Bech32Codec{
				Bech32Prefix: GetBech32AccountAddrPrefix(),
			},
			ValidatorAddressCodec: address.Bech32Codec{
				Bech32Prefix: GetBech32ValidatorAddrPrefix(),
			},
		},
	})
	appCodec := codec.NewProtoCodec(interfaceRegistry)
	legacyAmino := codec.NewLegacyAmino()
	txConfig := authtx.NewTxConfig(appCodec, authtx.DefaultSignModes)
	if err := interfaceRegistry.SigningContext().Validate(); err != nil {
		panic(err)
	}

	std.RegisterLegacyAminoCodec(legacyAmino)
	std.RegisterInterfaces(interfaceRegistry)
	baseAppOptions = append(baseAppOptions, baseapp.SetOptimisticExecution())

	bApp := baseapp.NewBaseApp(appName, logger, db, txConfig.TxDecoder(), baseAppOptions...)
	bApp.SetVersion(version.Version)
	bApp.SetInterfaceRegistry(interfaceRegistry)
	bApp.SetTxEncoder(txConfig.TxEncoder())

	keys := storetypes.NewKVStoreKeys(
		authtypes.StoreKey,
		banktypes.StoreKey,
		stakingtypes.StoreKey,
		distrtypes.StoreKey,
		slashingtypes.StoreKey,
		govtypes.StoreKey,
		consensusparamtypes.StoreKey,
		wasmtypes.StoreKey,
		// counter tutorial app wiring 3: add the counter store key below
	)

	if err := bApp.RegisterStreamingServices(appOpts, keys); err != nil {
		panic(err)
	}

	app := &PraxisApp{
		BaseApp:           bApp,
		legacyAmino:       legacyAmino,
		appCodec:          appCodec,
		txConfig:          txConfig,
		interfaceRegistry: interfaceRegistry,
		keys:              keys,
	}

	// set the BaseApp's parameter store
	app.ConsensusParamsKeeper = consensusparamkeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[consensusparamtypes.StoreKey]),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		runtime.EventService{},
	)
	bApp.SetParamStore(app.ConsensusParamsKeeper.ParamsStore)

	app.AccountKeeper = authkeeper.NewAccountKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		maccPerms,
		authcodec.NewBech32Codec(GetBech32MainPrefix()),
		GetBech32MainPrefix(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		authkeeper.WithUnorderedTransactions(true),
	)

	app.BankKeeper = bankkeeper.NewBaseKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		app.AccountKeeper,
		BlockedAddresses(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		logger,
	)

	app.StakingKeeper = stakingkeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[stakingtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		authcodec.NewBech32Codec(GetBech32PrefixValAddr()),
		authcodec.NewBech32Codec(GetBech32PrefixConsAddr()),
	)
	app.DistrKeeper = distrkeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[distrtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		app.StakingKeeper,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	app.SlashingKeeper = slashingkeeper.NewKeeper(
		appCodec,
		legacyAmino,
		runtime.NewKVStoreService(keys[slashingtypes.StoreKey]),
		app.StakingKeeper,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)
	app.StakingKeeper.SetHooks(
		stakingtypes.NewMultiStakingHooks(
			app.DistrKeeper.Hooks(),
			app.SlashingKeeper.Hooks(),
		),
	)
	// Register the proposal types
	// Deprecated: Avoid adding new handlers, instead use the new proposal flow
	// by granting the governance module the right to execute the message.
	// See: https://docs.cosmos.network/main/modules/gov#proposal-messages
	govRouter := govv1beta1.NewRouter()
	govRouter.AddRoute(govtypes.RouterKey, govv1beta1.ProposalHandler)
	govConfig := govtypes.DefaultConfig()
	/*
		Example of setting gov params:
		govConfig.MaxMetadataLen = 10000
	*/
	govKeeper := govkeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[govtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		app.DistrKeeper,
		app.MsgServiceRouter(),
		govConfig,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		govkeeper.NewDefaultCalculateVoteResultsAndVotingPower(app.StakingKeeper),
	)

	// Set legacy router for backwards compatibility with gov v1beta1
	govKeeper.SetLegacyRouter(govRouter)

	app.GovKeeper = *govKeeper.SetHooks(
		govtypes.NewMultiGovHooks(
		// register the governance hooks
		),
	)

	// wasm keeper
	//
	// NOTE: IBC is not enabled on this chain, so every IBC dependency of the wasm
	// keeper (ICS4 wrapper, channel keepers, transfer port source) is nil. contracts
	// can therefore not send IBC messages or run IBC queries. if IBC is added later,
	// wire the real keepers in here and use wasmkeeper.BuiltInCapabilities() as is
	homePath := cast.ToString(appOpts.Get(flags.FlagHome))
	wasmDir := filepath.Join(homePath, "wasm")
	nodeConfig, err := wasm.ReadNodeConfig(appOpts)
	if err != nil {
		panic(fmt.Sprintf("error while reading wasm config: %s", err))
	}

	app.WasmKeeper = wasmkeeper.NewKeeper(
		appCodec,
		runtime.NewKVStoreService(keys[wasmtypes.StoreKey]),
		app.AccountKeeper,
		app.BankKeeper,
		app.StakingKeeper,
		distrkeeper.NewQuerier(app.DistrKeeper),
		nil, // ICS4Wrapper
		nil, // ChannelKeeper
		nil, // ChannelKeeperV2
		nil, // ICS20TransferPortSource
		app.MsgServiceRouter(),
		app.GRPCQueryRouter(),
		wasmDir,
		nodeConfig,
		wasmtypes.VMConfig{},
		wasmCapabilities(),
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	)

	// counter tutorial app wiring 4: create the counter keeper below

	app.ModuleManager = module.NewManager(
		genutil.NewAppModule(
			app.AccountKeeper, app.StakingKeeper, app,
			txConfig,
		),
		auth.NewAppModule(appCodec, app.AccountKeeper, authsims.RandomGenesisAccounts),
		bank.NewAppModule(appCodec, app.BankKeeper, app.AccountKeeper),
		gov.NewAppModule(appCodec, &app.GovKeeper, app.AccountKeeper, app.BankKeeper),
		consensus.NewAppModule(appCodec, app.ConsensusParamsKeeper),
		slashing.NewAppModule(appCodec, app.SlashingKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper, app.interfaceRegistry),
		distr.NewAppModule(appCodec, app.DistrKeeper, app.AccountKeeper, app.BankKeeper, app.StakingKeeper),
		staking.NewAppModule(appCodec, app.StakingKeeper, app.AccountKeeper, app.BankKeeper),
		vesting.NewAppModule(app.AccountKeeper, app.BankKeeper),
		wasm.NewAppModule(appCodec, &app.WasmKeeper, app.StakingKeeper, app.AccountKeeper, app.BankKeeper, app.MsgServiceRouter()),
		// counter tutorial app wiring 5: register the counter module below
	)

	// BasicModuleManager defines the module BasicManager is in charge of setting up basic,
	// non-dependent module elements, such as codec registration and genesis verification.
	// By default it is composed of all the module from the module manager.
	// Additionally, app module basics can be overwritten by passing them as argument.
	app.BasicModuleManager = module.NewBasicManagerFromManager(
		app.ModuleManager,
		map[string]module.AppModuleBasic{
			genutiltypes.ModuleName: genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
			govtypes.ModuleName: gov.NewAppModuleBasic(
				[]govclient.ProposalHandler{},
			),
		},
	)

	app.BasicModuleManager.RegisterLegacyAminoCodec(legacyAmino)
	app.BasicModuleManager.RegisterInterfaces(interfaceRegistry)

	// create the simulation manager and define the order of the modules for deterministic simulations
	overrideModules := map[string]module.AppModuleSimulation{
		authtypes.ModuleName: auth.NewAppModule(appCodec, app.AccountKeeper, authsims.RandomGenesisAccounts),
	}
	app.sm = module.NewSimulationManagerFromAppModules(app.ModuleManager.Modules, overrideModules)
	app.sm.RegisterStoreDecoders()

	app.ModuleManager.SetOrderPreBlockers(
		authtypes.ModuleName,
	)
	app.ModuleManager.SetOrderBeginBlockers(
		distrtypes.ModuleName,
		slashingtypes.ModuleName,
		stakingtypes.ModuleName,
		genutiltypes.ModuleName,
	)
	app.ModuleManager.SetOrderEndBlockers(
		banktypes.ModuleName,
		govtypes.ModuleName,
		stakingtypes.ModuleName,
		genutiltypes.ModuleName,
	)
	// NOTE: The genutils module must occur after staking so that pools are
	// properly initialized with tokens from genesis accounts.
	// NOTE: The genutils module must also occur after auth so that it can access the params from auth.
	genesisModuleOrder := []string{
		authtypes.ModuleName,
		banktypes.ModuleName,
		distrtypes.ModuleName,
		stakingtypes.ModuleName,
		slashingtypes.ModuleName,
		govtypes.ModuleName,
		consensusparamtypes.ModuleName,
		vestingtypes.ModuleName,
		// counter tutorial app wiring 6: add the counter module to genesis order below
		genutiltypes.ModuleName,
		// wasm must come last: genesis messages can call into other modules
		// (bank, auth, staking, ...), which must be initialized by then.
		wasmtypes.ModuleName,
	}

	exportModuleOrder := []string{
		consensusparamtypes.ModuleName,
		authtypes.ModuleName,
		banktypes.ModuleName,
		distrtypes.ModuleName,
		stakingtypes.ModuleName,
		slashingtypes.ModuleName,
		govtypes.ModuleName,
		vestingtypes.ModuleName,
		// counter tutorial app wiring 7: add the counter module to export order below
		genutiltypes.ModuleName,
		wasmtypes.ModuleName,
	}

	app.ModuleManager.SetOrderInitGenesis(genesisModuleOrder...)
	app.ModuleManager.SetOrderExportGenesis(exportModuleOrder...)

	app.configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())
	err = app.ModuleManager.RegisterServices(app.configurator)
	if err != nil {
		panic(err)
	}
	autocliv1.RegisterQueryServer(app.GRPCQueryRouter(), runtimeservices.NewAutoCLIQueryService(app.ModuleManager.Modules))

	// initialize stores
	app.MountKVStores(keys)
	// initialize BaseApp
	app.SetInitChainer(app.InitChainer)
	app.SetPreBlocker(app.PreBlocker())
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetEndBlocker(app.EndBlocker)
	app.setAnteHandler(txConfig, nodeConfig, keys[wasmtypes.StoreKey])

	// must be before loading the version: the snapshot extension needs to be
	// registered before the store is loaded/restored.
	if manager := app.SnapshotManager(); manager != nil {
		if err := manager.RegisterExtensions(
			wasmkeeper.NewWasmSnapshotter(app.CommitMultiStore(), &app.WasmKeeper),
		); err != nil {
			panic(fmt.Errorf("failed to register snapshot extension: %w", err))
		}
	}

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			panic(fmt.Errorf("error loading last version: %w", err))
		}

		// pinned codes are not persisted in wasmvm, so they must be reinitialized on start
		ctx := app.NewContextLegacy(true, cmtproto.Header{})
		if err := app.WasmKeeper.InitializePinnedCodes(ctx); err != nil {
			panic(fmt.Errorf("failed to initialize pinned codes: %w", err))
		}
	}

	return app
}

// AutoCliOpts returns the autocli options for the app.
func (app *PraxisApp) AutoCliOpts() autocli.AppOptions {
	modules := make(map[string]appmodule.AppModule)
	for _, m := range app.ModuleManager.Modules {
		if moduleWithName, ok := m.(module.HasName); ok {
			moduleName := moduleWithName.Name()
			if appModule, ok := moduleWithName.(appmodule.AppModule); ok {
				modules[moduleName] = appModule
			}
		}
	}

	return autocli.AppOptions{
		Modules:               modules,
		ModuleOptions:         runtimeservices.ExtractAutoCLIOptions(app.ModuleManager.Modules),
		AddressCodec:          authcodec.NewBech32Codec(GetBech32AccountAddrPrefix()),
		ValidatorAddressCodec: authcodec.NewBech32Codec(GetBech32ValidatorAddrPrefix()),
		ConsensusAddressCodec: authcodec.NewBech32Codec(GetBech32ConsensusAddrPrefix()),
	}
}

func (app *PraxisApp) setAnteHandler(txConfig client.TxConfig, nodeConfig wasmtypes.NodeConfig, txCounterStoreKey *storetypes.KVStoreKey) {
	anteHandler, err := NewAnteHandler(
		HandlerOptions{
			HandlerOptions: ante.HandlerOptions{
				AccountKeeper:   app.AccountKeeper,
				BankKeeper:      app.BankKeeper,
				SignModeHandler: txConfig.SignModeHandler(),
				SigGasConsumer:  ante.DefaultSigVerificationGasConsumer,
				SigVerifyOptions: []ante.SigVerificationDecoratorOption{
					// change below as needed
					ante.WithUnorderedTxGasCost(ante.DefaultUnorderedTxGasCost),
					ante.WithMaxUnorderedTxTimeoutDuration(ante.DefaultMaxTimeoutDuration),
				},
			},
			NodeConfig:            &nodeConfig,
			WasmKeeper:            &app.WasmKeeper,
			TXCounterStoreService: runtime.NewKVStoreService(txCounterStoreKey),
		},
	)
	if err != nil {
		panic(err)
	}

	// Set the AnteHandler for the app
	app.SetAnteHandler(anteHandler)
}

// wasmCapabilities returns the cosmwasm capabilities supported by this chain
//
// IBC is not enabled, so the "ibc2" capability is left out: contracts that
// require it are rejected at upload time instead of failing at runtime
func wasmCapabilities() []string {
	caps := make([]string, 0, len(wasmkeeper.BuiltInCapabilities()))
	for _, c := range wasmkeeper.BuiltInCapabilities() {
		if c == "ibc2" {
			continue
		}
		caps = append(caps, c)
	}
	return caps
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *PraxisApp) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx
	// Register new tx routes from grpc-gateway.
	authtx.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register new CometBFT queries routes from grpc-gateway.
	cmtservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register node gRPC service for grpc-gateway.
	nodeservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register grpc-gateway routes for all modules.
	app.BasicModuleManager.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// register swagger API from root so that other applications can override easily
	if err := server.RegisterSwaggerAPI(apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger); err != nil {
		panic(err)
	}
}

// RegisterTxService implements the Application.RegisterTxService method.
func (app *PraxisApp) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.GRPCQueryRouter(), clientCtx, app.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *PraxisApp) RegisterTendermintService(clientCtx client.Context) {
	cmtApp := server.NewCometABCIWrapper(app)
	cmtservice.RegisterTendermintService(
		clientCtx,
		app.GRPCQueryRouter(),
		app.interfaceRegistry,
		cmtApp.Query,
	)
}

func (app *PraxisApp) RegisterNodeService(clientCtx client.Context, cfg config.Config) {
	nodeservice.RegisterNodeService(clientCtx, app.GRPCQueryRouter(), cfg, func() int64 {
		return app.CommitMultiStore().EarliestVersion()
	})
}

func (app *PraxisApp) Name() string {
	return appName
}

func (app *PraxisApp) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

func (app *PraxisApp) BeginBlocker(ctx sdk.Context) (sdk.BeginBlock, error) {
	return app.ModuleManager.BeginBlock(ctx)
}

func (app *PraxisApp) EndBlocker(ctx sdk.Context) (sdk.EndBlock, error) {
	return app.ModuleManager.EndBlock(ctx)
}

// GenesisState of the blockchain is represented here as a map of raw json
// messages key'd by a identifier string.
// The identifier is used to determine which module genesis information belongs
// to so it may be appropriately routed during init chain.
// Within this application default genesis information is retrieved from
// the ModuleBasicManager which populates json from each BasicModule
// object provided to it during init.
type GenesisState map[string]json.RawMessage

func (app *PraxisApp) InitChainer(ctx sdk.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
	var genesisState GenesisState
	if err := json.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}
	return app.ModuleManager.InitGenesis(ctx, app.appCodec, genesisState)
}

func (app *PraxisApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

func (app *PraxisApp) InterfaceRegistry() codectypes.InterfaceRegistry {
	return app.interfaceRegistry
}

func (app *PraxisApp) AppCodec() codec.Codec {
	return app.appCodec
}

func (app *PraxisApp) TxConfig() client.TxConfig {
	return app.txConfig
}

// BlockedAddresses returns all the app's blocked account addresses.
func BlockedAddresses() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range maccPerms {
		modAccAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}

	// allow the following addresses to receive funds
	delete(modAccAddrs, authtypes.NewModuleAddress(govtypes.ModuleName).String())

	return modAccAddrs
}

// GetKey returns the KVStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *PraxisApp) GetKey(storeKey string) *storetypes.KVStoreKey {
	return app.keys[storeKey]
}

func (app *PraxisApp) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

// DefaultGenesis returns a default genesis from the registered modules.
func (app *PraxisApp) DefaultGenesis() GenesisState {
	return app.BasicModuleManager.DefaultGenesis(app.appCodec)
}
