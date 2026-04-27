package tools

import (
	"fmt"
	"strings"

	binanceprovider "trading-go/tools/marketdata/binance"
	longbridgeprovider "trading-go/tools/marketdata/longbridge"
)

func NewProvider(providerName, defaultRegion string) (MarketDataProvider, string, error) {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "binance", "binance_futures":
		provider, err := binanceprovider.New()
		if err != nil {
			return nil, "", err
		}
		return provider, "binance_futures", nil
	case "", "auto", "longbridge":
		provider, err := longbridgeprovider.New(defaultRegion)
		if err != nil {
			return nil, "", err
		}
		return provider, "longbridge", nil
	default:
		return nil, "", fmt.Errorf("unsupported market data provider: %s", providerName)
	}
}
