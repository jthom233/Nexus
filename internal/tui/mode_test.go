package tui

import "testing"

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNormal, "NORMAL"},
		{ModeInsert, "INSERT"},
		{ModeVisual, "VISUAL"},
		{ModeCommand, "COMMAND"},
		{Mode(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestModeChangedMsg(t *testing.T) {
	msg := ModeChangedMsg{From: ModeNormal, To: ModeInsert}

	if msg.From != ModeNormal {
		t.Errorf("ModeChangedMsg.From = %v, want ModeNormal", msg.From)
	}
	if msg.To != ModeInsert {
		t.Errorf("ModeChangedMsg.To = %v, want ModeInsert", msg.To)
	}
}

func TestModeChangedMsgAllTransitions(t *testing.T) {
	transitions := []struct {
		from Mode
		to   Mode
	}{
		{ModeNormal, ModeInsert},
		{ModeNormal, ModeCommand},
		{ModeNormal, ModeVisual},
		{ModeInsert, ModeNormal},
		{ModeCommand, ModeNormal},
		{ModeVisual, ModeNormal},
	}

	for _, tt := range transitions {
		t.Run(tt.from.String()+"_to_"+tt.to.String(), func(t *testing.T) {
			msg := ModeChangedMsg{From: tt.from, To: tt.to}
			if msg.From != tt.from {
				t.Errorf("From = %v, want %v", msg.From, tt.from)
			}
			if msg.To != tt.to {
				t.Errorf("To = %v, want %v", msg.To, tt.to)
			}
		})
	}
}
