// Package i18n holds the UI string catalogs and the per-language template
// functions that read them.
//
// Catalogs live in locales/<code>.toml in go-i18n's message format. English is
// the base catalog: it is the source of truth for which keys exist, and every
// other language falls back to it key by key, so a partially translated locale
// renders English for the gaps rather than blanks or raw keys.
//
// Template functions are bound per language rather than taking the language as
// an argument. html/template resolves functions at execute time, so
// internal/templates parses each page set once and then clones it per language,
// overriding the func map on each clone with FuncMap(code). That keeps templates
// written as {{ t "key" }} with no language plumbing in the template data.
package i18n

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"hutzli.org/visoto/internal/logger"
)

// BaseCode is the source language: the catalog that defines the key set and the
// fallback every other locale resolves against.
const BaseCode = "en"

// missingMarker renders in place of a string that resolved nowhere: not in the
// requested language, not in the base catalog, and with no inline fallback at
// the call site. It is deliberately loud — a raw key reads like a label to
// anyone who does not know the codebase, whereas this cannot be mistaken for
// intended copy — and it still names the key, so a screenshot is enough to fix
// it without going to the log.
func missingMarker(id string) string {
	return "[⚠️translation is missing text block: " + id + "]"
}

// splitArgs separates the variadic tail of t/tHTML/tn into an optional inline
// fallback and optional template data.
//
// A string argument is the fallback; anything else is the data map. The two are
// unambiguous because data is always built with the dict helper, never written
// as a bare literal, so a string in that position can only be prose. Calls that
// pass neither — the overwhelming majority — get ("", nil) and behave exactly as
// they did before fallbacks existed.
func splitArgs(args []any) (fallback string, data any) {
	for _, a := range args {
		if s, ok := a.(string); ok && fallback == "" {
			fallback = s
			continue
		}
		if data == nil {
			data = a
		}
	}
	return fallback, data
}

// Catalogs is the loaded message catalogs plus one localizer per configured
// language. Build it once at startup with Load and treat it as immutable.
type Catalogs struct {
	bundle     *goi18n.Bundle
	localizers map[string]*goi18n.Localizer // keyed by lang.Key(code)
	keys       []string                     // every key in the base catalog, sorted
	defined    map[string]bool              // every key in ANY catalog; see defines
	warnedMu   sync.Mutex
	warned     map[string]bool
}

// Load reads locales/<code>.toml for every configured non-empty code and builds
// a localizer per code.
//
// A missing or unparseable base catalog is fatal — without it there are no
// strings at all. A missing translation catalog is a warning: the site still
// renders, in English, which is a far better failure mode than refusing to boot
// because one locale is behind.
func Load(dir string, codes []string) (*Catalogs, error) {
	bundle := goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	basePath := filepath.Join(dir, BaseCode+".toml")
	if _, err := bundle.LoadMessageFile(basePath); err != nil {
		return nil, fmt.Errorf("load base catalog %s: %w", basePath, err)
	}

	// Every key seen in any catalog, which is what tells an untranslated string
	// apart from one no catalog defines at all. The base catalog contributes only
	// the js.* keys now, so this has to span the translations too.
	defined := make(map[string]bool)
	for _, k := range baseKeys(basePath) {
		defined[k] = true
	}

	log := logger.Get()
	for _, code := range codes {
		if code == "" || code == BaseCode {
			continue
		}
		path := filepath.Join(dir, code+".toml")
		if _, err := os.Stat(path); err != nil {
			log.Warn("no message catalog for configured language; falling back to "+BaseCode,
				slog.String("language", code), slog.String("path", path))
			continue
		}
		if _, err := bundle.LoadMessageFile(path); err != nil {
			return nil, fmt.Errorf("load catalog %s: %w", path, err)
		}
		for _, k := range baseKeys(path) {
			defined[k] = true
		}
	}

	c := &Catalogs{
		bundle:     bundle,
		localizers: make(map[string]*goi18n.Localizer, len(codes)),
		defined:    defined,
		warned:     make(map[string]bool),
	}

	for _, code := range codes {
		// The "no language" choice has no catalog of its own; it renders the base
		// language, same as any locale with no translation for a key.
		tags := []string{BaseCode}
		if code != "" {
			tags = []string{code, BaseCode}
		}
		c.localizers[key(code)] = goi18n.NewLocalizer(bundle, tags...)
	}

	c.keys = baseKeys(basePath)
	log.Info("message catalogs loaded",
		slog.Int("languages", len(c.localizers)),
		slog.Int("keys", len(c.keys)))
	return c, nil
}

// key mirrors lang.Key without importing it, keeping this package free of a
// dependency it would otherwise only need for one three-line helper.
func key(code string) string {
	if code == "" {
		return "_"
	}
	return code
}

// baseKeys reads the base catalog again to recover its key list, which go-i18n's
// Bundle does not expose. Used by JSStrings to hand the frontend a catalog, and
// by tests that assert every key resolves in every language.
//
// Catalogs are written with quoted flat keys ("topbar.search" = "Search…"), but
// TOML also lets a dotted key be spelled as a nested table, and go-i18n accepts
// both — so this walks nested tables too and joins them the same way go-i18n
// does, or the two would disagree about which keys exist.
func baseKeys(path string) []string {
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil
	}
	var keys []string
	var walk func(prefix string, m map[string]any)
	walk = func(prefix string, m map[string]any) {
		for k, v := range m {
			id := k
			if prefix != "" {
				id = prefix + "." + k
			}
			sub, isTable := v.(map[string]any)
			if isTable && !isMessageTable(sub) {
				walk(id, sub)
				continue
			}
			keys = append(keys, id)
		}
	}
	walk("", raw)
	sort.Strings(keys)
	return keys
}

// messageFields are the field names go-i18n recognizes inside a message table.
// A table containing only these is one message (typically a plural form); a
// table containing anything else is a namespace of nested keys.
var messageFields = map[string]bool{
	"id": true, "description": true, "hash": true, "leftdelim": true, "rightdelim": true,
	"zero": true, "one": true, "two": true, "few": true, "many": true, "other": true,
}

func isMessageTable(m map[string]any) bool {
	if len(m) == 0 {
		return false
	}
	for k := range m {
		if !messageFields[strings.ToLower(k)] {
			return false
		}
	}
	return true
}

// Keys returns every message key defined by the base catalog.
func (c *Catalogs) Keys() []string {
	return append([]string(nil), c.keys...)
}

// defines reports whether any loaded catalog knows this key, in any language.
//
// It spans every language rather than just the base one because the base
// catalog no longer enumerates the template keys: their English lives inline at
// the call site, so a key is typically defined only in de/fr/it. Consulting
// c.keys alone would report every one of those as uncatalogued.
func (c *Catalogs) defines(id string) bool {
	return c.defined[id]
}

// translate resolves one key for one language.
//
// fallback is the inline English text from the call site, if it supplied any. It
// is a backstop only: a key present in any catalog always wins, matching how
// vsT behaves on the frontend. A key that resolves nowhere and has no fallback
// renders missingMarker, and is logged once per key/language.
func (c *Catalogs) translate(code, id, fallback string, data any) string {
	loc, ok := c.localizers[key(code)]
	if !ok {
		loc = c.localizers[key(BaseCode)]
	}
	if loc == nil {
		return c.finish(code, id, fallback, "", nil)
	}
	cfg := &goi18n.LocalizeConfig{MessageID: id}
	if data != nil {
		cfg.TemplateData = data
	}
	// Handing the fallback to go-i18n rather than returning it ourselves is what
	// makes {{.N}} work identically whether the text lives inline or in a
	// catalog: DefaultMessage goes through the same template renderer.
	if fallback != "" {
		cfg.DefaultMessage = &goi18n.Message{ID: id, Other: fallback}
	}
	msg, err := loc.Localize(cfg)
	return c.finish(code, id, fallback, msg, err)
}

// translateCount is translate for pluralized messages.
//
// The inline fallback fills only the Other form, so a fallback renders the same
// text for every count. That is a deliberate simplification: tn has no call
// sites that rely on a fallback, and proper CLDR pluralization resumes the
// moment a real catalog entry exists.
func (c *Catalogs) translateCount(code, id, fallback string, count any, data any) string {
	loc, ok := c.localizers[key(code)]
	if !ok {
		loc = c.localizers[key(BaseCode)]
	}
	if loc == nil {
		return c.finish(code, id, fallback, "", nil)
	}
	cfg := &goi18n.LocalizeConfig{MessageID: id, PluralCount: count}
	if fallback != "" {
		cfg.DefaultMessage = &goi18n.Message{ID: id, Other: fallback}
	}
	// Making the count available as {{.Count}} is what almost every plural string
	// actually wants, so seed it rather than making each call site pass it twice.
	merged := map[string]any{"Count": count}
	if m, ok := data.(map[string]any); ok {
		for k, v := range m {
			merged[k] = v
		}
	}
	cfg.TemplateData = merged
	msg, err := loc.Localize(cfg)
	return c.finish(code, id, fallback, msg, err)
}

// finish interprets a Localize result.
//
// go-i18n signals a per-key fallback by returning the base-language message
// *together with* a MessageNotFoundErr — the error means "not found in the
// requested language", not "not found at all". Treating any error as a miss
// would throw away every fallback string, so the message wins whenever it is
// non-empty and only a genuinely empty result degrades further.
//
// When an inline fallback was supplied, Localize has already rendered it via
// DefaultMessage, so msg holds the interpolated fallback and the two cases are
// distinguished by fallback being non-empty rather than by inspecting msg.
func (c *Catalogs) finish(code, id, fallback, msg string, err error) string {
	if msg != "" {
		// Whether the inline text ended up carrying this string cannot be read off
		// err: when DefaultMessage supplies the text, go-i18n reports success for
		// the base language and MessageNotFoundErr for every other one. So ask the
		// catalog directly instead.
		if fallback != "" && !c.defines(id) {
			// The call site's inline text is the only copy. How interesting that is
			// depends on the language:
			//
			// Base language — nothing anywhere defines this key. That is the
			// to-translate worklist, and it stays at INFO because the count is small
			// and actionable.
			//
			// Any other language — untranslated, which for a stub locale means every
			// string on every page. At INFO that buries the base-language signal
			// under hundreds of lines, so it drops to DEBUG: available when asked
			// for, absent from a normal run.
			if code == BaseCode || code == "" {
				c.logOnce(code, id, slog.LevelInfo, "no catalog entry; rendering inline fallback")
			} else {
				c.logOnce(code, id, slog.LevelDebug,
					"not translated; rendering "+BaseCode+" text from the template")
			}
		}
		return msg
	}
	if fallback != "" {
		// Reached only when Localize could not run at all (no localizer); the
		// fallback is then unrendered, which is still far better than a marker.
		return fallback
	}
	c.logOnce(code, id, slog.LevelWarn, "missing translation")
	return missingMarker(id)
}

// logOnce logs a key/language pair a single time, so a missing string in a hot
// partial cannot flood the log on every request.
func (c *Catalogs) logOnce(code, id string, level slog.Level, msg string) {
	k := code + "|" + id
	c.warnedMu.Lock()
	seen := c.warned[k]
	if !seen {
		c.warned[k] = true
	}
	c.warnedMu.Unlock()
	if seen {
		return
	}
	logger.Get().Log(context.Background(), level, msg,
		slog.String("language", code), slog.String("key", id))
}

// FuncMap returns the template functions bound to one language:
//
//	t    "key" [fallback] [data]        translated string
//	tHTML "key" [fallback] [data]       same, trusted as markup (inline tags)
//	tn   "key" count [fallback] [data]  plural form, with {{.Count}} pre-seeded
//
// fallback is the English text written inline at the call site. It keeps the
// template readable on its own and lets a string ship before its catalog entry
// exists; a catalog entry, once added, always takes precedence over it.
//
// data, when given, is a map — build it with the existing dict helper. Both
// optional arguments are told apart by type, so either or both may be omitted:
//
//	{{ t "resource.showingN" "Showing {{.N}} rows" (dict "N" 20) }}
func (c *Catalogs) FuncMap(code string) template.FuncMap {
	return template.FuncMap{
		"t": func(id string, args ...any) string {
			fallback, data := splitArgs(args)
			return c.translate(code, id, fallback, data)
		},
		// Only for strings that intentionally contain markup. Catalog content and
		// inline fallbacks are both authored in-repo, never user input, so trusting
		// them is safe — but routing ordinary strings through here would defeat
		// escaping, so keep its use deliberate.
		"tHTML": func(id string, args ...any) template.HTML {
			fallback, data := splitArgs(args)
			return template.HTML(c.translate(code, id, fallback, data))
		},
		"tn": func(id string, count any, args ...any) string {
			fallback, data := splitArgs(args)
			return c.translateCount(code, id, fallback, count, data)
		},
		// The active language code, for <html lang> and the topbar picker.
		"siteLang": func() string { return code },
		// The js.* subset, for the JSON island base.html hands to window.vsT.
		"jsStrings": func() map[string]string { return c.JSStrings(code) },
	}
}

// JSStrings returns the subset of the catalog whose keys start with "js." ,
// translated into code. base.html serializes this into a JSON island that
// static/js reads through window.vsT — the frontend's equivalent of {{ t }}.
// Keeping the frontend strings inside the page body also keeps them covered by
// the response ETag.
func (c *Catalogs) JSStrings(code string) map[string]string {
	out := make(map[string]string)
	for _, k := range c.keys {
		if strings.HasPrefix(k, "js.") {
			out[k] = c.translate(code, k, "", nil)
		}
	}
	return out
}
