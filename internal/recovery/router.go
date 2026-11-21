// Package recovery implements router recovery adapters and model selection.
package recovery

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RouterModelAskeyRTF8316VW is the stable ROUTER_MODEL identifier for the
// currently supported Askey RTF8316VW firmware adapter.
const RouterModelAskeyRTF8316VW = "askey-rtf8316vw"

// DefaultRouterModel preserves the original Askey recovery behavior when
// ROUTER_MODEL is omitted from existing deployments.
const DefaultRouterModel = RouterModelAskeyRTF8316VW

// RouterRecovery is implemented by each router-specific recovery adapter.
// Keep vendor/firmware HTTP details behind this interface.
type RouterRecovery interface {
	Recover(context.Context) error
}

// RouterConfig holds configuration for a router recovery adapter.
type RouterConfig struct {
	Model                    string
	URL, Username, Password  string
	DNSPrimary, DNSSecondary string
	WiFiOff                  time.Duration
	FailureThreshold         int
	Cooldown                 time.Duration
}

// NormalizeRouterModel converts the supplied model identifier into its
// canonical lower-case slug form.
func NormalizeRouterModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return DefaultRouterModel
	}
	return model
}

func unsupportedRouterModelError(model string) error {
	return fmt.Errorf("unsupported ROUTER_MODEL %q; supported models: %s, %s (see CONTRIBUTING.md to add an adapter)", model, RouterModelAskeyRTF8316VW, RouterModelAsusGeneric)
}

// ValidateRouterConfig checks that cfg selects a supported adapter and that
// adapter-specific settings are valid, without creating a client or cookie jar.
func ValidateRouterConfig(cfg RouterConfig) error {
	model := NormalizeRouterModel(cfg.Model)
	switch model {
	case RouterModelAskeyRTF8316VW:
		return nil
	case RouterModelAsusGeneric:
		_, err := parseAsusBaseURL(cfg.URL)
		return err
	default:
		return unsupportedRouterModelError(model)
	}
}

// NewRouterRecovery selects the adapter for ROUTER_MODEL. Unknown models are
// rejected instead of accidentally sending Askey-specific requests to another
// manufacturer's router.
func NewRouterRecovery(cfg RouterConfig) (RouterRecovery, error) {
	model := NormalizeRouterModel(cfg.Model)
	cfg.Model = model
	switch model {
	case RouterModelAskeyRTF8316VW:
		return NewAskeyRouter(cfg), nil
	case RouterModelAsusGeneric:
		return newAsusGenericRouter(cfg)
	default:
		return nil, unsupportedRouterModelError(model)
	}
}
