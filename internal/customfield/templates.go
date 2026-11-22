package customfield

import (
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// Built-in item templates per project type, in English and Swedish. Their
// ids are "builtin:<project type>:<name>"; they cannot be changed, but a
// project or organisation can copy one and edit the copy.

// BuiltinPrefix starts the id of every built-in template.
const BuiltinPrefix = "builtin:"

type tr struct{ en, sv string }

func (t tr) in(lang string) string {
	if lang == "sv" && t.sv != "" {
		return t.sv
	}
	return t.en
}

type crit struct {
	desc      tr
	method    string
	threshold tr
}

type step struct{ action, expected, data tr }

type builtin struct {
	projectType, kind, slug string
	name, description       tr
	title, body             tr
	obligation, reqType     string
	priority                string
	tags                    []string
	criteria                []crit
	preconditions, expected tr
	steps                   []step
	estimate                int
}

var builtins = []builtin{
	// ---------------------------------------------------------------- general
	{projectType: "general", kind: model.KindRequirement, slug: "requirement",
		name:        tr{"Requirement with acceptance criteria", "Krav med acceptanskriterier"},
		description: tr{"A plain requirement statement with acceptance criteria.", "Ett enkelt kravuttryck med acceptanskriterier."},
		title:       tr{"<Subject> shall <capability>", "<Subjekt> ska <förmåga>"},
		body: tr{"The <subject> shall <capability> so that <benefit>.\n\n## Context\n<Why is this needed?>",
			"<Subjektet> ska <förmåga> så att <nytta>.\n\n## Sammanhang\n<Varför behövs detta?>"},
		obligation: model.ObligationShall, reqType: "functional", priority: "medium",
		criteria: []crit{{tr{"<Observable condition that shows the requirement is met>", "<Observerbart villkor som visar att kravet är uppfyllt>"}, "test", tr{}}}},
	{projectType: "general", kind: model.KindTest, slug: "manual-test",
		name:          tr{"Manual test", "Manuellt test"},
		description:   tr{"Preconditions, steps and an expected result.", "Förutsättningar, steg och förväntat resultat."},
		title:         tr{"Verify <behaviour>", "Verifiera <beteende>"},
		preconditions: tr{"<State of the system before the test>", "<Systemets tillstånd före testet>"},
		expected:      tr{"<Overall expected outcome>", "<Förväntat utfall som helhet>"},
		steps:         []step{{tr{"<Action>", "<Åtgärd>"}, tr{"<Expected result>", "<Förväntat resultat>"}, tr{}}},
		estimate:      15},

	// ---------------------------------------------------------------- software
	{projectType: "software", kind: model.KindRequirement, slug: "user-story",
		name:        tr{"Functional requirement (user story)", "Funktionellt krav (användarberättelse)"},
		description: tr{"As a … I want … so that …, with Given/When/Then acceptance criteria.", "Som … vill jag … så att …, med Givet/När/Då-acceptanskriterier."},
		title:       tr{"<Role> can <goal>", "<Roll> kan <mål>"},
		body: tr{"**As a** <role>\n**I want** <goal>\n**so that** <benefit>.\n\n## Notes\n<Business rules, examples, out of scope>",
			"**Som** <roll>\n**vill jag** <mål>\n**så att** <nytta>.\n\n## Anteckningar\n<Affärsregler, exempel, utanför omfattningen>"},
		obligation: model.ObligationShall, reqType: "functional", priority: "medium", tags: []string{"user-story"},
		criteria: []crit{
			{tr{"Given <context>, when <action>, then <outcome>", "Givet <sammanhang>, när <åtgärd>, då <utfall>"}, "test", tr{}},
			{tr{"Given <context>, when <invalid action>, then <error is shown>", "Givet <sammanhang>, när <felaktig åtgärd>, då <visas ett fel>"}, "test", tr{}},
		}},
	{projectType: "software", kind: model.KindRequirement, slug: "non-functional",
		name:        tr{"Non-functional requirement (measurable)", "Icke-funktionellt krav (mätbart)"},
		description: tr{"A quality requirement with a measurable threshold and how it is measured.", "Ett kvalitetskrav med ett mätbart gränsvärde och hur det mäts."},
		title:       tr{"<Quality> of <function> within <limit>", "<Kvalitet> för <funktion> inom <gräns>"},
		body: tr{"The system shall <quality attribute> for <function> under <load or conditions>.\n\n## Measurement\n- Metric: <e.g. 95th percentile response time>\n- Method: <tool, environment>\n- Threshold: <value and unit>",
			"Systemet ska <kvalitetsegenskap> för <funktion> under <last eller villkor>.\n\n## Mätning\n- Mått: <t.ex. 95:e percentilens svarstid>\n- Metod: <verktyg, miljö>\n- Gränsvärde: <värde och enhet>"},
		obligation: model.ObligationShall, reqType: "performance", priority: "high", tags: []string{"nfr"},
		criteria: []crit{{tr{"95th percentile response time under nominal load", "95:e percentilens svarstid vid nominell last"}, "test", tr{"≤ 500 ms", "≤ 500 ms"}},
			{tr{"Availability per calendar month", "Tillgänglighet per kalendermånad"}, "analysis", tr{"≥ 99.9 %", "≥ 99,9 %"}}}},
	{projectType: "software", kind: model.KindTest, slug: "api-test",
		name:          tr{"API test (request/response)", "API-test (anrop/svar)"},
		description:   tr{"Steps that send requests and check status, headers and body.", "Steg som skickar anrop och kontrollerar status, rubriker och innehåll."},
		title:         tr{"API: <method> <path> <behaviour>", "API: <metod> <sökväg> <beteende>"},
		preconditions: tr{"- Test environment reachable at <base URL>\n- Valid API token for a user with <role>", "- Testmiljön nås på <bas-URL>\n- Giltig API-token för en användare med <roll>"},
		expected:      tr{"The API answers as specified and changes nothing else.", "API:et svarar enligt specifikationen och ändrar inget annat."},
		steps: []step{
			{tr{"Send `POST /api/<resource>` with a valid body", "Skicka `POST /api/<resurs>` med ett giltigt innehåll"}, tr{"201 Created; body contains the new id; `Location` header set", "201 Created; svaret innehåller nytt id; rubriken `Location` är satt"}, tr{`{"name": "example"}`, `{"name": "exempel"}`}},
			{tr{"Send `GET /api/<resource>/{id}`", "Skicka `GET /api/<resurs>/{id}`"}, tr{"200 OK; the body equals what was sent", "200 OK; innehållet är det som skickades"}, tr{}},
			{tr{"Send `POST /api/<resource>` with a missing required field", "Skicka `POST /api/<resurs>` utan ett obligatoriskt fält"}, tr{"400 Bad Request with an error message naming the field", "400 Bad Request med ett felmeddelande som nämner fältet"}, tr{}},
			{tr{"Send the request without a token", "Skicka anropet utan token"}, tr{"401 Unauthorized", "401 Unauthorized"}, tr{}},
		},
		estimate: 10},

	// ---------------------------------------------------------------- hardware
	{projectType: "hardware", kind: model.KindRequirement, slug: "design-tolerance",
		name:        tr{"Design requirement with tolerance", "Konstruktionskrav med tolerans"},
		description: tr{"A dimension or characteristic with nominal value, tolerance and verification.", "Ett mått eller en egenskap med nominellt värde, tolerans och verifiering."},
		title:       tr{"<Part>: <characteristic> <nominal> ± <tolerance>", "<Detalj>: <egenskap> <nominellt> ± <tolerans>"},
		body: tr{"The <part> shall have a <characteristic> of <nominal value> <unit> ± <tolerance> <unit>.\n\n| Nominal | Tolerance | Unit | Reference |\n|---|---|---|---|\n| <value> | ± <value> | <unit> | <drawing / standard> |",
			"<Detaljen> ska ha <egenskap> <nominellt värde> <enhet> ± <tolerans> <enhet>.\n\n| Nominellt | Tolerans | Enhet | Referens |\n|---|---|---|---|\n| <värde> | ± <värde> | <enhet> | <ritning / standard> |"},
		obligation: model.ObligationShall, reqType: "constraint", priority: "high", tags: []string{"design"},
		criteria: []crit{{tr{"Measured <characteristic> is within the tolerance on every sample", "Uppmätt <egenskap> ligger inom toleransen på varje prov"}, "inspection", tr{"<nominal> ± <tolerance> <unit>", "<nominellt> ± <tolerans> <enhet>"}}}},
	{projectType: "hardware", kind: model.KindTest, slug: "environmental",
		name:          tr{"Environmental test (IEC 60068 style)", "Miljötest (enligt IEC 60068)"},
		description:   tr{"Initial measurement, conditioning, exposure, recovery and final measurement.", "Inledande mätning, konditionering, exponering, återhämtning och slutmätning."},
		title:         tr{"Environmental test: <test, e.g. Db damp heat cyclic> on <unit>", "Miljötest: <test, t.ex. Db fuktig värme cyklisk> på <enhet>"},
		preconditions: tr{"- Test specimen: <part number, serial>\n- Test standard: IEC 60068-2-<part>\n- Chamber calibrated (certificate <no.>)", "- Provobjekt: <artikelnummer, serienummer>\n- Teststandard: IEC 60068-2-<del>\n- Klimatkammare kalibrerad (certifikat <nr>)"},
		expected:      tr{"No damage; functional and performance values within specification after recovery.", "Inga skador; funktion och prestanda inom specifikation efter återhämtning."},
		steps: []step{
			{tr{"Initial measurements: visual inspection and functional test", "Inledande mätningar: okulärbesiktning och funktionstest"}, tr{"Specimen passes the functional test; no visible damage", "Provobjektet klarar funktionstestet; inga synliga skador"}, tr{}},
			{tr{"Pre-conditioning at standard atmospheric conditions", "Förkonditionering i standardatmosfär"}, tr{"Specimen stabilised", "Provobjektet stabiliserat"}, tr{"15–35 °C, 25–75 % RH, ≥ 2 h", "15–35 °C, 25–75 % RF, ≥ 2 h"}},
			{tr{"Conditioning (exposure) according to the test severity", "Konditionering (exponering) enligt provningsstränghet"}, tr{"Chamber profile recorded within tolerance; specimen <operating/not operating>", "Kammarprofil registrerad inom tolerans; provobjekt <i drift/ej i drift>"}, tr{"<temperature, humidity, duration, cycles>", "<temperatur, fuktighet, varaktighet, cykler>"}},
			{tr{"Intermediate measurements during exposure (if specified)", "Mellanmätningar under exponering (om specificerat)"}, tr{"Values within specification", "Värden inom specifikation"}, tr{}},
			{tr{"Recovery at standard atmospheric conditions", "Återhämtning i standardatmosfär"}, tr{"Specimen stabilised", "Provobjektet stabiliserat"}, tr{"1–2 h", "1–2 h"}},
			{tr{"Final measurements: visual inspection and functional test", "Slutmätningar: okulärbesiktning och funktionstest"}, tr{"Same results as the initial measurements; no corrosion or damage", "Samma resultat som vid inledande mätningar; ingen korrosion eller skada"}, tr{}},
		},
		estimate: 480},
	{projectType: "hardware", kind: model.KindTest, slug: "emc",
		name:          tr{"EMC test", "EMC-test"},
		description:   tr{"Emission or immunity test with set-up, levels and performance criterion.", "Emissions- eller immunitetstest med uppställning, nivåer och funktionskriterium."},
		title:         tr{"EMC: <phenomenon, e.g. radiated immunity IEC 61000-4-3> on <unit>", "EMC: <fenomen, t.ex. strålad immunitet IEC 61000-4-3> på <enhet>"},
		preconditions: tr{"- EUT: <product, configuration, cables>\n- Test site: <semi-anechoic chamber / GTEM>\n- Performance criterion: <A / B / C>", "- EUT: <produkt, konfiguration, kablar>\n- Testplats: <semiekofritt rum / GTEM>\n- Funktionskriterium: <A / B / C>"},
		expected:      tr{"Emissions below the limits / EUT meets the performance criterion.", "Emissioner under gränsvärdena / EUT uppfyller funktionskriteriet."},
		steps: []step{
			{tr{"Set up the EUT and auxiliary equipment as in the test plan", "Ställ upp EUT och hjälputrustning enligt testplanen"}, tr{"Set-up documented with photos", "Uppställningen dokumenterad med foton"}, tr{}},
			{tr{"Operate the EUT in its worst-case mode", "Kör EUT i det mest ogynnsamma driftläget"}, tr{"EUT works normally", "EUT fungerar normalt"}, tr{}},
			{tr{"Apply the test level / sweep the frequency range", "Applicera provnivån / svep frekvensområdet"}, tr{"Levels recorded; EUT monitored", "Nivåer registrerade; EUT övervakas"}, tr{"<e.g. 80 MHz–6 GHz, 10 V/m, 80 % AM 1 kHz>", "<t.ex. 80 MHz–6 GHz, 10 V/m, 80 % AM 1 kHz>"}},
			{tr{"Check the EUT's function during and after the test", "Kontrollera EUT:s funktion under och efter testet"}, tr{"Performance criterion met", "Funktionskriteriet uppfyllt"}, tr{}},
		},
		estimate: 240},

	// ---------------------------------------------------------------- system
	{projectType: "system", kind: model.KindRequirement, slug: "interface",
		name:        tr{"Interface requirement", "Gränssnittskrav"},
		description: tr{"What crosses an interface between two elements, in which format and with which timing.", "Vad som passerar ett gränssnitt mellan två delar, i vilket format och med vilken tidsåtgång."},
		title:       tr{"<Element A> shall provide <data/signal> to <Element B>", "<Del A> ska tillhandahålla <data/signal> till <Del B>"},
		body: tr{"<Element A> shall provide <data, signal or service> to <Element B> over <interface / protocol>.\n\n| Item | Format | Range | Rate | Latency |\n|---|---|---|---|---|\n| <name> | <type, unit> | <min–max> | <Hz> | <ms> |\n\n## Interface control document\n<ICD reference and version>",
			"<Del A> ska tillhandahålla <data, signal eller tjänst> till <Del B> via <gränssnitt / protokoll>.\n\n| Post | Format | Område | Takt | Latens |\n|---|---|---|---|---|\n| <namn> | <typ, enhet> | <min–max> | <Hz> | <ms> |\n\n## Gränssnittsbeskrivning\n<ICD-referens och version>"},
		obligation: model.ObligationShall, reqType: "interface", priority: "high", tags: []string{"interface"},
		criteria: []crit{{tr{"Messages conform to the ICD", "Meddelandena följer gränssnittsbeskrivningen"}, "test", tr{"100 % of sampled messages", "100 % av stickprovsmeddelandena"}},
			{tr{"End-to-end latency", "Latens från ände till ände"}, "test", tr{"≤ <value> ms", "≤ <värde> ms"}}}},
	{projectType: "system", kind: model.KindTest, slug: "integration",
		name:          tr{"Integration test", "Integrationstest"},
		description:   tr{"Checks that two or more integrated elements work together across their interfaces.", "Kontrollerar att två eller flera integrerade delar fungerar tillsammans över sina gränssnitt."},
		title:         tr{"Integration: <Element A> ↔ <Element B>", "Integration: <Del A> ↔ <Del B>"},
		preconditions: tr{"- <Element A> version <x> and <Element B> version <y> installed\n- Interface monitor connected", "- <Del A> version <x> och <Del B> version <y> installerade\n- Gränssnittsmonitor ansluten"},
		expected:      tr{"Data is exchanged as specified; errors are detected and reported.", "Data utbyts enligt specifikationen; fel upptäcks och rapporteras."},
		steps: []step{
			{tr{"Start both elements and establish the connection", "Starta båda delarna och upprätta förbindelsen"}, tr{"Connection established; no errors logged", "Förbindelsen upprättad; inga fel loggade"}, tr{}},
			{tr{"Trigger <event> in <Element A>", "Utlös <händelse> i <Del A>"}, tr{"<Element B> receives <data> within the specified latency", "<Del B> tar emot <data> inom angiven latens"}, tr{}},
			{tr{"Interrupt the interface", "Bryt gränssnittet"}, tr{"Both elements detect the loss and report it", "Båda delarna upptäcker avbrottet och rapporterar det"}, tr{}},
			{tr{"Restore the interface", "Återställ gränssnittet"}, tr{"Communication resumes without manual intervention", "Kommunikationen återupptas utan manuellt ingrepp"}, tr{}},
		},
		estimate: 60},

	// ---------------------------------------------------------------- business
	{projectType: "business", kind: model.KindRequirement, slug: "business-rule",
		name:        tr{"Business rule", "Affärsregel"},
		description: tr{"A rule of the business with its condition, action, exceptions and owner.", "En verksamhetsregel med villkor, åtgärd, undantag och ägare."},
		title:       tr{"Rule: <short name>", "Regel: <kort namn>"},
		body: tr{"**When** <condition>\n**then** <action or constraint>.\n\n## Exceptions\n<cases where the rule does not apply>\n\n## Examples\n| Input | Result |\n|---|---|\n| <example> | <result> |",
			"**När** <villkor>\n**då** <åtgärd eller begränsning>.\n\n## Undantag\n<fall då regeln inte gäller>\n\n## Exempel\n| Indata | Resultat |\n|---|---|\n| <exempel> | <resultat> |"},
		obligation: model.ObligationMust, reqType: "business", priority: "medium", tags: []string{"business-rule"},
		criteria: []crit{{tr{"Every example in the table gives the stated result", "Varje exempel i tabellen ger angivet resultat"}, "test", tr{}}}},
	{projectType: "business", kind: model.KindTest, slug: "acceptance",
		name:          tr{"Acceptance test", "Acceptanstest"},
		description:   tr{"A business scenario the customer or product owner signs off.", "Ett verksamhetsscenario som kunden eller produktägaren godkänner."},
		title:         tr{"Acceptance: <business scenario>", "Acceptans: <verksamhetsscenario>"},
		preconditions: tr{"- Accepted by: <customer / product owner>\n- Test data: <customers, orders, …>", "- Godkänns av: <kund / produktägare>\n- Testdata: <kunder, ordrar, …>"},
		expected:      tr{"The scenario delivers the business outcome; the acceptor signs off.", "Scenariot ger det avsedda verksamhetsutfallet; godkännaren skriver under."},
		steps: []step{
			{tr{"Given <business context>", "Givet <verksamhetssammanhang>"}, tr{"Context is in place", "Sammanhanget finns på plats"}, tr{}},
			{tr{"When <user performs the business task>", "När <användaren utför verksamhetsuppgiften>"}, tr{"The task can be completed without help", "Uppgiften kan slutföras utan hjälp"}, tr{}},
			{tr{"Then <business outcome is checked>", "Då <kontrolleras verksamhetsutfallet>"}, tr{"Outcome matches the acceptance criteria", "Utfallet stämmer med acceptanskriterierna"}, tr{}},
			{tr{"Acceptor reviews the result", "Godkännaren granskar resultatet"}, tr{"Signed off (name, date)", "Godkänt (namn, datum)"}, tr{}},
		},
		estimate: 30},
}

// Builtins returns the built-in templates of a project type and item kind
// ("" for every kind) in the language lang (en or sv).
func Builtins(projectType, kind, lang string) []model.ItemTemplate {
	var out []model.ItemTemplate
	for i, b := range builtins {
		if b.projectType != projectType || (kind != "" && b.kind != kind) {
			continue
		}
		out = append(out, b.template(lang, i))
	}
	return out
}

// Builtin returns a built-in template by id.
func Builtin(id, lang string) (model.ItemTemplate, bool) {
	for i, b := range builtins {
		if b.id() == id {
			return b.template(lang, i), true
		}
	}
	return model.ItemTemplate{}, false
}

// IsBuiltin reports whether id names a built-in template.
func IsBuiltin(id string) bool { return strings.HasPrefix(id, BuiltinPrefix) }

func (b builtin) id() string { return BuiltinPrefix + b.projectType + ":" + b.slug }

func (b builtin) template(lang string, pos int) model.ItemTemplate {
	t := model.ItemTemplate{ID: b.id(), ProjectType: b.projectType, Kind: b.kind, Name: b.name.in(lang),
		Description: b.description.in(lang), Source: model.TemplateBuiltin, Position: pos}
	c := &t.Content
	c.Title, c.Description, c.Priority = b.title.in(lang), b.body.in(lang), b.priority
	c.Tags = append([]string(nil), b.tags...)
	if b.kind == model.KindRequirement {
		c.Obligation, c.Type, c.Status = b.obligation, b.reqType, "draft"
		for _, cr := range b.criteria {
			c.Criteria = append(c.Criteria, model.EvaluationCriterion{Description: cr.desc.in(lang), Method: cr.method, Threshold: cr.threshold.in(lang)})
		}
	} else {
		c.Status, c.ExecutionType, c.EstimatedMin = "draft", model.ExecManual, b.estimate
		c.Preconditions, c.ExpectedResult = b.preconditions.in(lang), b.expected.in(lang)
		for _, st := range b.steps {
			c.Steps = append(c.Steps, model.TestStep{Action: st.action.in(lang), Expected: st.expected.in(lang), Data: st.data.in(lang)})
		}
	}
	return t
}
