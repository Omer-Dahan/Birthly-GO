// Package callbacks encodes and decodes Telegram callback_data payloads.
//
// Every callback_data string in the bot comes from one of the Encode
// functions here, never a hand-built string, so payloads stay within
// Telegram's 64-byte limit and stay structurally validated — port of
// app/callbacks/factories.py.
//
// Wire format: "<prefix>:<field1>:<field2>:...", colon-separated, matching
// aiogram's CallbackData default separator. An empty field position encodes
// as an empty string segment ("::"), decoded back to nil/absent — matching
// aiogram's handling of Optional[str]/Optional[int] fields.
package callbacks

import (
	"fmt"
	"strconv"
	"strings"
)

const sep = ":"

func join(parts ...string) string {
	return strings.Join(parts, sep)
}

func optStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func optInt(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func parseOptStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseOptInt(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ── mnu: MenuCallback ───────────────────────────────────────────────────

const PrefixMenu = "mnu"

type Menu struct{ Action string }

func (c Menu) Encode() string { return join(PrefixMenu, c.Action) }

func DecodeMenu(data string) (Menu, error) {
	parts, err := split(data, PrefixMenu, 1)
	if err != nil {
		return Menu{}, err
	}
	return Menu{Action: parts[0]}, nil
}

// ── nav: NavCallback ────────────────────────────────────────────────────

const PrefixNav = "nav"

type Nav struct{ Action string }

func (c Nav) Encode() string { return join(PrefixNav, c.Action) }

func DecodeNav(data string) (Nav, error) {
	parts, err := split(data, PrefixNav, 1)
	if err != nil {
		return Nav{}, err
	}
	return Nav{Action: parts[0]}, nil
}

// ── noop: NoopCallback ──────────────────────────────────────────────────

const PrefixNoop = "noop"

func EncodeNoop() string { return PrefixNoop }

// ── ev: EventFlowCallback / EventCallback share prefix "ev" ────────────
//
// aiogram tells these two apart at the router filter level (F.action.in_(...))
// rather than by payload shape, since both are two-field "ev:x:y" strings.
// Go has no equivalent implicit-filter dispatch, so DecodeEvent below makes
// the split explicit: action tokens are looked up in a fixed table to decide
// whether the second field is a free-form value (flow family) or a numeric
// event_id (card family) — get this table wrong and the two callback
// families silently collide.

const PrefixEvent = "ev"

// eventFlowActions: add/edit-flow actions with no event id yet.
var eventFlowActions = map[string]bool{
	"type": true, "heb": true, "hm": true, "hd": true, "gm": true, "gd": true,
	"noyear": true, "cat": true, "gender": true, "more": true, "field": true,
	"clear": true, "skip": true, "save": true, "cancel": true,
	"secyes": true, "secno": true,
}

// eventCardActions: card-scoped actions on an existing, owned event.
var eventCardActions = map[string]bool{
	"v": true, "e": true, "d": true, "dy": true, "undo": true,
	"gr": true, "sh": true, "rem": true, "mute": true,
}

// EventFlow is the add/edit-flow shape: action + optional free-form value.
type EventFlow struct {
	Action string
	Value  *string
}

func (c EventFlow) Encode() string {
	if !eventFlowActions[c.Action] {
		panic("callbacks: " + c.Action + " is not a registered EventFlow action")
	}
	return join(PrefixEvent, c.Action, optStr(c.Value))
}

// EventCard is the card-scoped shape: action + a concrete owned event id.
type EventCard struct {
	Action  string
	EventID int64
}

func (c EventCard) Encode() string {
	if !eventCardActions[c.Action] {
		panic("callbacks: " + c.Action + " is not a registered EventCard action")
	}
	return join(PrefixEvent, c.Action, strconv.FormatInt(c.EventID, 10))
}

// EventDecoded is the result of DecodeEvent: exactly one of Flow/Card is set.
type EventDecoded struct {
	Flow *EventFlow
	Card *EventCard
}

// DecodeEvent decodes an "ev:..." callback_data payload, routing to the
// EventFlow or EventCard shape by action token (see package doc — this is
// the fix for the two Python classes sharing a prefix).
func DecodeEvent(data string) (EventDecoded, error) {
	parts, err := split(data, PrefixEvent, 2)
	if err != nil {
		return EventDecoded{}, err
	}
	action, second := parts[0], parts[1]

	switch {
	case eventFlowActions[action]:
		return EventDecoded{Flow: &EventFlow{Action: action, Value: parseOptStr(second)}}, nil
	case eventCardActions[action]:
		id, err := strconv.ParseInt(second, 10, 64)
		if err != nil {
			return EventDecoded{}, fmt.Errorf("callbacks: ev card action %q: invalid event_id %q: %w", action, second, err)
		}
		return EventDecoded{Card: &EventCard{Action: action, EventID: id}}, nil
	default:
		return EventDecoded{}, fmt.Errorf("callbacks: unknown ev action %q", action)
	}
}

// ── ls: ListCallback ────────────────────────────────────────────────────

const PrefixList = "ls"

type List struct {
	Action string // p | sort | filt | fset | view
	Value  string
}

func (c List) Encode() string { return join(PrefixList, c.Action, c.Value) }

func DecodeList(data string) (List, error) {
	parts, err := split(data, PrefixList, 2)
	if err != nil {
		return List{}, err
	}
	return List{Action: parts[0], Value: parts[1]}, nil
}

// ── rem: ReminderCallback ───────────────────────────────────────────────

const PrefixReminder = "rem"

type Reminder struct {
	Action  string // add | off | time | tog | del
	Value   *string
	EventID *int64
	// RuleID is only set for the "time" action, which needs to carry both
	// the chosen time (in Value) and the rule being changed — packing both
	// into Value (as "HH-MM:ruleID") used to collide with this format's
	// colon separator and made every time-picker button undecodable.
	RuleID *int64
}

func (c Reminder) Encode() string {
	return join(PrefixReminder, c.Action, optStr(c.Value), optInt(c.EventID), optInt(c.RuleID))
}

func DecodeReminder(data string) (Reminder, error) {
	parts, err := split(data, PrefixReminder, 4)
	if err != nil {
		return Reminder{}, err
	}
	eventID, err := parseOptInt(parts[2])
	if err != nil {
		return Reminder{}, fmt.Errorf("callbacks: rem event_id: %w", err)
	}
	ruleID, err := parseOptInt(parts[3])
	if err != nil {
		return Reminder{}, fmt.Errorf("callbacks: rem rule_id: %w", err)
	}
	return Reminder{Action: parts[0], Value: parseOptStr(parts[1]), EventID: eventID, RuleID: ruleID}, nil
}

// ── set: SettingsCallback ───────────────────────────────────────────────

const PrefixSettings = "set"

type Settings struct {
	Action string
	Value  *string
}

func (c Settings) Encode() string { return join(PrefixSettings, c.Action, optStr(c.Value)) }

func DecodeSettings(data string) (Settings, error) {
	parts, err := split(data, PrefixSettings, 2)
	if err != nil {
		return Settings{}, err
	}
	return Settings{Action: parts[0], Value: parseOptStr(parts[1])}, nil
}

// ── st: StatsCallback ───────────────────────────────────────────────────

const PrefixStats = "st"

type Stats struct{ Action string } // home | months | cats | ages

func (c Stats) Encode() string { return join(PrefixStats, c.Action) }

func DecodeStats(data string) (Stats, error) {
	parts, err := split(data, PrefixStats, 1)
	if err != nil {
		return Stats{}, err
	}
	return Stats{Action: parts[0]}, nil
}

// ── tpl: TemplateCallback ───────────────────────────────────────────────

const PrefixTemplate = "tpl"

type Template struct {
	Action    string // style | pick | ai | list | use | new | new_tone | del
	Value     *string
	EventID   *int64
	ExcludeID *int64
}

func (c Template) Encode() string {
	return join(PrefixTemplate, c.Action, optStr(c.Value), optInt(c.EventID), optInt(c.ExcludeID))
}

func DecodeTemplate(data string) (Template, error) {
	parts, err := split(data, PrefixTemplate, 4)
	if err != nil {
		return Template{}, err
	}
	eventID, err := parseOptInt(parts[2])
	if err != nil {
		return Template{}, fmt.Errorf("callbacks: tpl event_id: %w", err)
	}
	excludeID, err := parseOptInt(parts[3])
	if err != nil {
		return Template{}, fmt.Errorf("callbacks: tpl exclude_id: %w", err)
	}
	return Template{Action: parts[0], Value: parseOptStr(parts[1]), EventID: eventID, ExcludeID: excludeID}, nil
}

// ── bk: BackupCallback ──────────────────────────────────────────────────

const PrefixBackup = "bk"

type Backup struct {
	Action string // home | exp | imp | auto
	Value  *string
}

func (c Backup) Encode() string { return join(PrefixBackup, c.Action, optStr(c.Value)) }

func DecodeBackup(data string) (Backup, error) {
	parts, err := split(data, PrefixBackup, 2)
	if err != nil {
		return Backup{}, err
	}
	return Backup{Action: parts[0], Value: parseOptStr(parts[1])}, nil
}

// ── adm: AdminCallback ──────────────────────────────────────────────────

const PrefixAdmin = "adm"

type Admin struct {
	Action string // home | stats | bc | bc_ok | logs | u
	Value  *string
}

func (c Admin) Encode() string { return join(PrefixAdmin, c.Action, optStr(c.Value)) }

func DecodeAdmin(data string) (Admin, error) {
	parts, err := split(data, PrefixAdmin, 2)
	if err != nil {
		return Admin{}, err
	}
	return Admin{Action: parts[0], Value: parseOptStr(parts[1])}, nil
}

// ── shared decode helper ────────────────────────────────────────────────

// Prefix returns the prefix segment of a callback_data string, for routing
// to the right Decode* function before any field parsing.
func Prefix(data string) string {
	prefix, _, _ := strings.Cut(data, sep)
	return prefix
}

func split(data, wantPrefix string, wantFields int) ([]string, error) {
	parts := strings.Split(data, sep)
	if len(parts) != wantFields+1 {
		return nil, fmt.Errorf("callbacks: %q: expected prefix+%d fields, got %d segments", data, wantFields, len(parts))
	}
	if parts[0] != wantPrefix {
		return nil, fmt.Errorf("callbacks: %q: expected prefix %q, got %q", data, wantPrefix, parts[0])
	}
	return parts[1:], nil
}
