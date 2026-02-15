package termcap

import (
	"testing"
)

func TestParseIsDark(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantDark bool
	}{
		{
			name:     "black is dark",
			input:    "rgb:0000/0000/0000",
			wantDark: true,
		},
		{
			name:     "white is light",
			input:    "rgb:ffff/ffff/ffff",
			wantDark: false,
		},
		{
			name:     "typical dark theme",
			input:    "rgb:1a1b/2c3d/3e4f",
			wantDark: true,
		},
		{
			name:     "typical light theme",
			input:    "rgb:e0e0/e0e0/e0e0",
			wantDark: false,
		},
		{
			name:     "invalid input defaults to dark",
			input:    "not-a-color",
			wantDark: true,
		},
		{
			name:     "empty string defaults to dark",
			input:    "",
			wantDark: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIsDark(tt.input)
			if got != tt.wantDark {
				t.Errorf("parseIsDark(%q) = %v, want %v", tt.input, got, tt.wantDark)
			}
		})
	}
}

func TestProbeColorProfile(t *testing.T) {
	tests := []struct {
		name      string
		colorterm string
		term      string
		want      string
	}{
		{
			name:      "COLORTERM truecolor",
			colorterm: "truecolor",
			term:      "",
			want:      "truecolor",
		},
		{
			name:      "COLORTERM 24bit",
			colorterm: "24bit",
			term:      "",
			want:      "truecolor",
		},
		{
			name:      "TERM xterm-256color",
			colorterm: "",
			term:      "xterm-256color",
			want:      "256",
		},
		{
			name:      "TERM xterm",
			colorterm: "",
			term:      "xterm",
			want:      "16",
		},
		{
			name:      "TERM dumb",
			colorterm: "",
			term:      "dumb",
			want:      "mono",
		},
		{
			name:      "both empty",
			colorterm: "",
			term:      "",
			want:      "mono",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv sets the var for this test and restores it afterward
			t.Setenv("COLORTERM", tt.colorterm)
			t.Setenv("TERM", tt.term)

			got := probeColorProfile()
			if got != tt.want {
				t.Errorf("probeColorProfile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProbeOSC52(t *testing.T) {
	tests := []struct {
		name        string
		termProgram string
		tmux        string
		term        string
		want        bool
	}{
		{
			name:        "ghostty supports OSC52",
			termProgram: "ghostty",
			tmux:        "",
			term:        "",
			want:        true,
		},
		{
			name:        "kitty supports OSC52",
			termProgram: "kitty",
			tmux:        "",
			term:        "",
			want:        true,
		},
		{
			name:        "tmux supports OSC52",
			termProgram: "",
			tmux:        "/tmp/tmux-1000/default,12345,0",
			term:        "",
			want:        true,
		},
		{
			name:        "screen term prefix supports OSC52",
			termProgram: "",
			tmux:        "",
			term:        "screen-256color",
			want:        true,
		},
		{
			name:        "xterm prefix supports OSC52",
			termProgram: "",
			tmux:        "",
			term:        "xterm-256color",
			want:        true,
		},
		{
			name:        "dumb terminal does not support OSC52",
			termProgram: "",
			tmux:        "",
			term:        "dumb",
			want:        false,
		},
		{
			name:        "all unset does not support OSC52",
			termProgram: "",
			tmux:        "",
			term:        "",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", tt.termProgram)
			t.Setenv("TMUX", tt.tmux)
			t.Setenv("TERM", tt.term)

			got := probeOSC52()
			if got != tt.want {
				t.Errorf("probeOSC52() = %v, want %v", got, tt.want)
			}
		})
	}
}
