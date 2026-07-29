package callbacks

import "testing"

func TestEventFlowVsCardDisambiguation(t *testing.T) {
	// The trap from the plan: EventFlowCallback and EventCallback share the
	// "ev" prefix. A "save" action must decode as Flow; a "v" (view) action
	// on a numeric id must decode as Card.
	flowData := EventFlow{Action: "save", Value: nil}.Encode()
	decoded, err := DecodeEvent(flowData)
	if err != nil {
		t.Fatalf("DecodeEvent(%q): %v", flowData, err)
	}
	if decoded.Flow == nil || decoded.Card != nil {
		t.Fatalf("DecodeEvent(%q) = %+v, want Flow set, Card nil", flowData, decoded)
	}
	if decoded.Flow.Action != "save" {
		t.Errorf("Flow.Action = %q, want save", decoded.Flow.Action)
	}

	cardData := EventCard{Action: "v", EventID: 42}.Encode()
	decoded2, err := DecodeEvent(cardData)
	if err != nil {
		t.Fatalf("DecodeEvent(%q): %v", cardData, err)
	}
	if decoded2.Card == nil || decoded2.Flow != nil {
		t.Fatalf("DecodeEvent(%q) = %+v, want Card set, Flow nil", cardData, decoded2)
	}
	if decoded2.Card.EventID != 42 {
		t.Errorf("Card.EventID = %d, want 42", decoded2.Card.EventID)
	}
}

func TestEventFlowWithValue(t *testing.T) {
	val := "birthday"
	data := EventFlow{Action: "type", Value: &val}.Encode()
	decoded, err := DecodeEvent(data)
	if err != nil {
		t.Fatalf("DecodeEvent(%q): %v", data, err)
	}
	if decoded.Flow == nil || decoded.Flow.Value == nil || *decoded.Flow.Value != "birthday" {
		t.Errorf("DecodeEvent(%q) = %+v, want Flow.Value=birthday", data, decoded)
	}
}

func TestReminderRoundTrip(t *testing.T) {
	val := "add"
	eventID := int64(7)
	data := Reminder{Action: "add", Value: &val, EventID: &eventID}.Encode()
	decoded, err := DecodeReminder(data)
	if err != nil {
		t.Fatalf("DecodeReminder(%q): %v", data, err)
	}
	if decoded.Action != "add" || decoded.Value == nil || *decoded.Value != "add" || decoded.EventID == nil || *decoded.EventID != 7 {
		t.Errorf("DecodeReminder(%q) = %+v, want add/add/7", data, decoded)
	}

	// nil fields round-trip to nil, not "0" or empty-string-as-value.
	data2 := Reminder{Action: "tog", Value: nil, EventID: nil}.Encode()
	decoded2, err := DecodeReminder(data2)
	if err != nil {
		t.Fatalf("DecodeReminder(%q): %v", data2, err)
	}
	if decoded2.Value != nil || decoded2.EventID != nil {
		t.Errorf("DecodeReminder(%q) = %+v, want both nil", data2, decoded2)
	}
}

func TestTemplateRoundTrip(t *testing.T) {
	eventID := int64(3)
	data := Template{Action: "use", EventID: &eventID}.Encode()
	decoded, err := DecodeTemplate(data)
	if err != nil {
		t.Fatalf("DecodeTemplate(%q): %v", data, err)
	}
	if decoded.Action != "use" || decoded.EventID == nil || *decoded.EventID != 3 || decoded.ExcludeID != nil {
		t.Errorf("DecodeTemplate(%q) = %+v", data, decoded)
	}
}

// TestTemplateRoundTrip_WithExcludeID guards a specific regression: the
// Python original broke when packing a "pick another" callback because
// exclude_id collided with the callback-data field separator. Every field
// (Action, Value, EventID, ExcludeID) must survive encode->decode intact
// when ExcludeID is set alongside the others, not just when it's absent.
func TestTemplateRoundTrip_WithExcludeID(t *testing.T) {
	eventID := int64(3)
	excludeID := int64(17)
	value := "warm"
	data := Template{Action: "pick", Value: &value, EventID: &eventID, ExcludeID: &excludeID}.Encode()

	decoded, err := DecodeTemplate(data)
	if err != nil {
		t.Fatalf("DecodeTemplate(%q): %v", data, err)
	}
	if decoded.Action != "pick" {
		t.Errorf("Action = %q, want pick", decoded.Action)
	}
	if decoded.Value == nil || *decoded.Value != "warm" {
		t.Errorf("Value = %v, want warm", decoded.Value)
	}
	if decoded.EventID == nil || *decoded.EventID != 3 {
		t.Errorf("EventID = %v, want 3", decoded.EventID)
	}
	if decoded.ExcludeID == nil || *decoded.ExcludeID != 17 {
		t.Errorf("ExcludeID = %v, want 17", decoded.ExcludeID)
	}
}

func TestPrefixRouting(t *testing.T) {
	cases := map[string]string{
		Menu{Action: "home"}.Encode():          PrefixMenu,
		Nav{Action: "back"}.Encode():           PrefixNav,
		EncodeNoop():                           PrefixNoop,
		List{Action: "p", Value: "2"}.Encode(): PrefixList,
		Stats{Action: "home"}.Encode():         PrefixStats,
	}
	for data, want := range cases {
		if got := Prefix(data); got != want {
			t.Errorf("Prefix(%q) = %q, want %q", data, got, want)
		}
	}
}

func TestDecodeEventUnknownActionErrors(t *testing.T) {
	if _, err := DecodeEvent("ev:bogus:1"); err == nil {
		t.Error("DecodeEvent with unknown action should error")
	}
}

func TestDecodeWrongPrefixErrors(t *testing.T) {
	if _, err := DecodeMenu("nav:home"); err == nil {
		t.Error("DecodeMenu on a nav: payload should error")
	}
}
