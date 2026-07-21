package process

import (
	"log/slog"
	"testing"
)

func TestPopupMsg_NoPanic(t *testing.T) {
	ctrl := NewWithConfig("test-app", "", "", false, 0, slog.Default())

	// Should not panic and return nil on any platform
	if err := ctrl.PopupMsg("Test Title", "Test Message"); err != nil {
		t.Errorf("PopupMsg error = %v, want nil", err)
	}
	if err := ctrl.PopupMsg("Just Title", "Just Title"); err != nil {
		t.Errorf("PopupMsg error = %v, want nil", err)
	}
	if err := ctrl.PopupMsg("", ""); err != nil {
		t.Errorf("PopupMsg error = %v, want nil", err)
	}
}
