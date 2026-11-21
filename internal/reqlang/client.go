package reqlang

// The web client highlights obligation keywords in requirement texts
// without a round trip to the server. It does so with the keyword tables of
// this package, exported by KeywordTable into
// web/src/lib/obligationKeywords.json; a test keeps that file in sync (run
// `go test ./internal/reqlang -run TestClientTable -update` after changing
// a language pack).

// ClientKeyword is one obligation phrase of a language pack, as matched by
// the client: words separated by whitespace, case-insensitive unless
// CaseSensitive, on word boundaries, longest phrase first (the order of
// the list).
type ClientKeyword struct {
	Text          string `json:"text"`
	Level         string `json:"level"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
}

// ClientPack is the part of a language pack the client needs to detect the
// language and find the obligation keywords.
type ClientPack struct {
	Code string `json:"code"`
	// UppercaseRule: only uppercase keywords are normative (RFC 8174).
	UppercaseRule bool            `json:"uppercase_rule"`
	Keywords      []ClientKeyword `json:"keywords"`
	Markers       []string        `json:"markers"`
	Letters       string          `json:"letters,omitempty"`
	// Examples are keywords to suggest when a text has none.
	Examples string `json:"examples"`
}

// ClientTable is the keyword table of all registered packs.
type ClientTable struct {
	// Levels from strongest to weakest (see Levels).
	Levels []string     `json:"levels"`
	Packs  []ClientPack `json:"packs"`
}

// KeywordTable returns the keyword table of the registered language packs,
// sorted by language code.
func KeywordTable() ClientTable {
	t := ClientTable{Levels: append([]string{}, Levels...), Packs: []ClientPack{}}
	for _, code := range Languages() {
		pk := packs[code]
		cp := ClientPack{Code: code, UppercaseRule: pk.uppercaseRule, Markers: append([]string{}, pk.markerList...), Letters: pk.letters, Examples: pk.examples}
		for _, p := range pk.keywords {
			cp.Keywords = append(cp.Keywords, ClientKeyword{Text: p.text, Level: p.level, CaseSensitive: p.caseSensitive})
		}
		t.Packs = append(t.Packs, cp)
	}
	return t
}
