package recovery

import "testing"

func TestValidateRouterConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     RouterConfig
		wantErr bool
	}{
		{name: "askey", cfg: RouterConfig{Model: RouterModelAskeyRTF8316VW}},
		{name: "default model is askey", cfg: RouterConfig{}},
		{name: "asus http", cfg: RouterConfig{Model: RouterModelAsusGeneric, URL: "http://192.168.1.1"}},
		{name: "asus https", cfg: RouterConfig{Model: RouterModelAsusGeneric, URL: "https://192.168.1.1/"}},
		{name: "asus invalid scheme", cfg: RouterConfig{Model: RouterModelAsusGeneric, URL: "ftp://192.168.1.1"}, wantErr: true},
		{name: "asus empty url", cfg: RouterConfig{Model: RouterModelAsusGeneric}, wantErr: true},
		{name: "unsupported model", cfg: RouterConfig{Model: "unknown-router"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRouterConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateRouterConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
