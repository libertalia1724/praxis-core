package app

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func GetBech32AccountAddrPrefix() string {
	return "praxis"
}

func GetBech32ValidatorAddrPrefix() string {
	return "praxisvaloper"
}

func GetBech32MainPrefix() string {
	return "praxis"
}

func GetBech32PrefixValAddr() string {
	return "praxisvaloper"
}

func GetBech32PrefixConsAddr() string {
	return "praxisvalcons"
}

func GetBech32ConsensusAddrPrefix() string {
	return "praxisvalcons"
}

func GetMicroPraxisDenom() string {
	return "upraxis"
}

func SetConfig() {
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(GetBech32AccountAddrPrefix(), "praxispub")
	cfg.SetBech32PrefixForValidator(GetBech32ValidatorAddrPrefix(), "praxisvaloperpub")
	cfg.SetBech32PrefixForConsensusNode(GetBech32ConsensusAddrPrefix(), "praxisvalconspub")
	cfg.Seal()
}
