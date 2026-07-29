package callbacks

import "testing"

// FuzzDecodeAll feeds arbitrary strings to every Decode* function.
// callback_data is genuinely adversarial input, unlike most other parsing
// in this codebase: Telegram does not verify a callback_data payload
// actually belongs to a button this bot sent, so a modified client can send
// any byte string here. A panic in any decoder would crash update
// processing (recovered by errorReporter.handlePanic since that fix, but
// far better to never panic in the first place) — the only thing this test
// asserts is "never panics", since every one of these functions is designed
// to return an error for malformed input, not to succeed meaningfully on
// fuzzed data.
func FuzzDecodeAll(f *testing.F) {
	seeds := []string{
		"", ":", "::::::::", "mnu:home", "ev:v:123", "ls:p:-1",
		"rem:t:18-00:42", "set:lang:en", "tpl:use:1:2:3", "bk:exp:json",
		"adm:stats", "nav:back", "stat:home", "mnu", "mnu:", ":::",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("decoding %q panicked: %v", data, r)
			}
		}()
		_, _ = DecodeMenu(data)
		_, _ = DecodeNav(data)
		_, _ = DecodeEvent(data)
		_, _ = DecodeList(data)
		_, _ = DecodeReminder(data)
		_, _ = DecodeSettings(data)
		_, _ = DecodeStats(data)
		_, _ = DecodeTemplate(data)
		_, _ = DecodeBackup(data)
		_, _ = DecodeAdmin(data)
	})
}
