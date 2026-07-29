package handlers

import "testing"

func TestFallbackMessageText(t *testing.T) {
	if got := fallbackMessageText(true); got != fallbackWithStateText {
		t.Errorf("fallbackMessageText(true) = %q, want the in-flow guidance text", got)
	}
	if got := fallbackMessageText(false); got != fallbackWithoutStateText {
		t.Errorf("fallbackMessageText(false) = %q, want the no-flow guidance text", got)
	}
}

func TestAnyMessageAndAnyCallbackAlwaysMatch(t *testing.T) {
	if !anyMessage(nil) {
		t.Error("anyMessage should always return true, even for a nil message")
	}
	if !anyCallback(nil) {
		t.Error("anyCallback should always return true, even for a nil callback query")
	}
}
