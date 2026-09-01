package image

import "testing"

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetectFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantOK  bool // env conclusive
		wantSup bool // supported
		proto   Protocol
	}{
		{"kitty via TERM", map[string]string{"TERM": "xterm-kitty"}, true, true, ProtocolKitty},
		{"kitty via KITTY_WINDOW_ID", map[string]string{"KITTY_WINDOW_ID": "1"}, true, true, ProtocolKitty},
		{"ghostty via TERM", map[string]string{"TERM": "xterm-ghostty"}, true, true, ProtocolKitty},
		{"ghostty via TERM_PROGRAM", map[string]string{"TERM_PROGRAM": "ghostty"}, true, true, ProtocolKitty},
		{"ghostty via resources dir", map[string]string{"GHOSTTY_RESOURCES_DIR": "/x"}, true, true, ProtocolKitty},
		{"wezterm", map[string]string{"WEZTERM_EXECUTABLE": "/usr/bin/wezterm"}, true, true, ProtocolKitty},
		{"wezterm via TERM_PROGRAM", map[string]string{"TERM_PROGRAM": "WezTerm"}, true, true, ProtocolKitty},
		{"konsole new enough", map[string]string{"KONSOLE_VERSION": "220400"}, true, true, ProtocolKitty},
		{"konsole too old", map[string]string{"KONSOLE_VERSION": "220399"}, true, false, ProtocolNone},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, false, false, ProtocolNone},
		{"tmux no hints", map[string]string{"TERM": "screen-256color"}, false, false, ProtocolNone},
		{"empty", map[string]string{}, false, false, ProtocolNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap, ok := detectFromEnv(envFunc(tt.env))
			if ok != tt.wantOK {
				t.Fatalf("conclusive = %v, want %v", ok, tt.wantOK)
			}
			if cap.Supported != tt.wantSup {
				t.Fatalf("supported = %v, want %v (reason %q)", cap.Supported, tt.wantSup, cap.Reason)
			}
			if cap.Protocol != tt.proto {
				t.Fatalf("protocol = %q, want %q", cap.Protocol, tt.proto)
			}
		})
	}
}

func TestDetect_UnknownEnvNoProbe(t *testing.T) {
	cap := Detect(envFunc(map[string]string{"TERM": "xterm-256color"}), false)
	if cap.Supported {
		t.Fatalf("unknown terminal without probe must be unsupported, got %+v", cap)
	}
}

func TestDetect_KnownEnvSkipsProbe(t *testing.T) {
	// activeProbe true, but env is conclusive so no I/O should be attempted.
	cap := Detect(envFunc(map[string]string{"TERM": "xterm-kitty"}), false)
	if !cap.Supported || cap.Protocol != ProtocolKitty {
		t.Fatalf("kitty env should be supported, got %+v", cap)
	}
}

func TestCapabilityCellSizeFallback(t *testing.T) {
	w, h := Capability{}.CellSize()
	if w != fallbackCellW || h != fallbackCellH {
		t.Fatalf("fallback cell size = %dx%d", w, h)
	}
	w, h = Capability{CellW: 10, CellH: 21}.CellSize()
	if w != 10 || h != 21 {
		t.Fatalf("explicit cell size not preserved: %dx%d", w, h)
	}
}
