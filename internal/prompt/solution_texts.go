package prompt

// The texts of the solution prompts per language. Titles get the project;
// tasks the number of requirements; partTitle (title, part, parts);
// continuation texts (part, parts, split mode or earlier parts).

type solLang struct {
	lang         string
	s            map[string]string
	labels       map[string]string // preamble field labels
	styles       map[string]string // architecture styles
	levels       map[string]string // risk levels
	instructions map[string][]string
	common       []string
	formats      map[string]string
	answers      map[string][]string
	tail         []string
}

var solTexts = map[string]*solLang{"en": solEnglish, "sv": solSwedish}

// PreambleLabel is the label of a preamble field in a language.
func PreambleLabel(lang, field string) string {
	if tx := solTexts[lang]; tx != nil {
		if l, ok := tx.labels[field]; ok {
			return l
		}
	}
	return solEnglish.labels[field]
}

var solEnglish = &solLang{
	lang: "en",
	s: map[string]string{
		"title.architecture":        "Architecture of %[1]s",
		"title.design":              "Design of %[1]s",
		"title.implementation_plan": "Implementation plan for %[1]s",
		"title.full_solution":       "Solution for %[1]s: architecture, design and implementation plan",
		"partTitle":                 "%[1]s — part %[2]d of %[3]d",

		"role.architecture":        "You are an experienced software architect. You design solutions that meet the requirements within the constraints of the solution preamble, and you make the trade-offs explicit.",
		"role.design":              "You are an experienced software designer and senior engineer who turns requirements and an architecture into a detailed design that can be implemented directly.",
		"role.implementation_plan": "You are an experienced technical lead who plans implementation work so that every task is traceable to the requirements and \"done\" means verified.",
		"role.full_solution":       "You are an experienced software architect and technical lead. You produce a complete solution — architecture, detailed design and implementation plan — traceable to the requirements.",

		"task.architecture":        "Propose the architecture of the solution for the requirements below (%[1]d). Do not write the implementation.",
		"task.design":              "Produce the design of the solution for the requirements below (%[1]d), consistent with the solution preamble.",
		"task.implementation_plan": "Break the implementation of the requirements below (%[1]d) into milestones, epics and tasks, in a workable order.",
		"task.full_solution":       "Produce the architecture, the design and the implementation plan for the requirements below (%[1]d), in one answer.",

		"h.preamble":     "Solution preamble",
		"h.summary":      "Project summary",
		"h.order":        "Dependency order",
		"h.general":      "For all parts",
		"h.continuation": "Continuation",
		"noPreamble":     "No solution preamble was given: state the technologies, environments and constraints you assume, and keep them consistent.",
		"wholeProject":   "the whole project",
		"scope.selected": "%[1]d selected requirements",
		"scope.query":    "query",
		"scope.folder":   "folder",
		"scope.release":  "release",
		"scope.variant":  "variant",
		"noFolder":       "No folder",
		"compactNote":    "Compact mode: only titles, obligations, priorities and acceptance criteria are given.",
		"orderIntro":     "The requirements with dependencies between them, prerequisites first (from the \"depends on\" relations):",
		"cycle":          "Dependency cycle, decide the order",

		"sum.scope":       "Scope: %[1]s — %[2]d requirements (of %[3]d in the project)",
		"sum.part":        "This part: %[1]d of %[2]d, with %[3]d requirements",
		"sum.obligations": "Requirements by obligation",
		"sum.folders":     "Folders",
		"sum.releases":    "Releases",
		"sum.variants":    "Variants",
		"sum.links":       "Linked risks: %[1]d · verifying tests: %[2]d (of %[3]d tests in the project)",

		"f.folder": "Folder", "f.risks": "Risks", "f.level": "level", "f.mitigation": "mitigation",
		"f.standards": "Standards and frameworks",

		"part.architecture":        "Architecture",
		"part.design":              "Design",
		"part.implementation_plan": "Implementation plan",

		"by.folder": "top-level folder",
		"by.size":   "size",
		"by.none":   "size",

		"cont.first":  "This is part %[1]d of %[2]d of one prompt; the requirements are split by %[3]s and the next parts follow. Answer this part only. Give elements, decisions and tasks stable names and numbers: the next parts continue them. Give the traceability rows for this part's requirements; the consolidated table comes with the last part.",
		"cont.middle": "This is part %[1]d of %[2]d. Continue the same answer as the earlier parts (%[3]s): reuse the names and numbers you introduced there, do not repeat earlier sections, add only what these requirements need, and refer to existing elements instead of defining them again. Give the traceability rows for this part's requirements.",
		"cont.last":   "This is the last part (%[1]d of %[2]d); the earlier parts covered %[3]s. Continue as before, then finish with the consolidated traceability table for the requirements of all parts and the complete lists of assumptions and open questions.",

		"traceHeader":  "| Requirement | Covered by (elements, decisions, tasks) | Verifying tests | Status |",
		"traceExample": "| KEY-1 Title | e.g. component, ADR-1, endpoint, task T3 | test keys or \"to write\" | covered / partly / not covered / question |",
	},
	labels: map[string]string{
		"languages":          "Implementation languages and versions",
		"frameworks":         "Frameworks and libraries",
		"environments":       "Target environments",
		"architecture_style": "Architecture style",
		"architecture_notes": "Architecture notes",
		"frontend":           "Frontend",
		"backend":            "Backend",
		"messaging":          "Messaging",
		"caching":            "Caching",
		"databases":          "Databases (described; no credentials)",
		"integrations":       "Integrations and external APIs",
		"deployment":         "Deployment and CI/CD",
		"performance":        "Performance targets",
		"availability":       "Availability targets",
		"scalability":        "Scalability targets",
		"security":           "Security and compliance constraints",
		"conventions":        "Coding conventions",
		"testing":            "Testing strategy",
		"repository":         "Repository layout",
		"out_of_scope":       "Out of scope",
		"additional":         "Additional context",
	},
	styles: map[string]string{
		"monolith": "monolith", "modular_monolith": "modular monolith", "microservices": "microservices", "serverless": "serverless",
		"event_driven": "event-driven", "layered": "layered", "hexagonal": "hexagonal (ports and adapters)", "other": "other (see the notes)",
	},
	levels: map[string]string{"critical": "critical", "high": "high", "medium": "medium", "low": "low"},
	instructions: map[string][]string{
		Architecture: {
			"Describe the system context: the users and roles, the external systems (the integrations of the preamble and those the requirements name) and what crosses the system boundary.",
			"Describe the containers (deployable or runnable units, data stores, message channels) and the main components of each, with their responsibilities and the technologies of the preamble.",
			"Record the key architecture decisions as ADR candidates: context, decision, alternatives considered, consequences, and the requirement keys that drive each decision.",
			"Write quality attribute scenarios (source, stimulus, environment, response, response measure) for the non-functional requirements and the non-functional targets of the preamble, each mapped to requirement keys.",
			"List the architectural risks and technical debt, with the linked project risks where they apply, and how the architecture mitigates them.",
			"Keep to the architecture style, target environments and out-of-scope items of the preamble; where a requirement does not fit them, say so instead of changing the constraints.",
		},
		Design: {
			"Design the modules and components: responsibilities, public interfaces and dependencies, consistent with the architecture style and the repository layout of the preamble.",
			"Derive the data model from the requirements: entities, attributes, keys, relationships and constraints (an entity-relationship model).",
			"Design the API: resources or operations, request and response shapes, status and error codes, pagination, versioning and the authorisation of each operation.",
			"Describe the sequence flows of the key use cases (the MUST requirements first), step by step across the components.",
			"Define the error handling: validation, expected failures, retries and idempotency, timeouts, and what the user or caller sees.",
			"Design the security: authentication, authorisation, input validation, protection of data at rest and in transit, auditing, and the mitigations of the linked risks and of the mapped CWE, ATT&CK, D3FEND and EMB3D items.",
		},
		ImplementationPlan: {
			"Break the work into milestones, epics and tasks; every task names the requirement keys it implements or supports and is small enough for one person in a few days.",
			"Order the work by the dependencies between the requirements (\"depends on\", \"refines\"; see the dependency order), then by obligation and priority: MUST before SHOULD before MAY.",
			"Give each epic a definition of done: its acceptance criteria met, its verifying tests (by key and title) passing, and the reviews, documentation and deployment steps the preamble requires.",
			"Name the tests to write where a requirement has no verifying test yet.",
			"Show what can run in parallel and what blocks other work; flag the risks to the plan.",
		},
	},
	common: []string{
		"Trace every design element (component, decision, entity, endpoint, flow, task) to the requirement keys it serves, and list the requirements that no element covers.",
		"Do not invent requirements or behaviour: flag ambiguities, gaps and conflicts between requirements (including \"conflicts with\" relations) as open questions, with a proposed resolution.",
		"List every assumption you make, numbered (A1, A2 …), and refer to it where it is used.",
		"Use the technologies, versions and conventions of the solution preamble; propose an alternative only with a reason.",
		"MUST requirements and their acceptance criteria are not negotiable; SHOULD requirements may be deferred with a reason; MAY requirements are optional.",
		"Never put passwords, keys or other secrets in the answer; refer to where they are kept.",
	},
	formats: map[string]string{
		"markdown":    "Write the answer in Markdown, with the numbered sections of the answer format as headings.",
		FormatMermaid: "Draw diagrams as Mermaid code blocks (```mermaid): flowcharts or component diagrams, sequence diagrams for the flows, erDiagram for the data model.",
		FormatC4:      "Show the system context and the containers in the C4 model (Mermaid C4Context and C4Container diagrams).",
		FormatOpenAPI: "Give the API as an OpenAPI 3.1 skeleton in YAML (paths, operations, schemas, error responses, security schemes) with the requirement keys in x-requirements.",
		FormatSQL:     "Give the data model as SQL DDL (CREATE TABLE with keys, constraints and indexes) for the database engine of the preamble, with the requirement keys in comments.",
	},
	answers: map[string][]string{
		Architecture:       {"System context", "Containers and components", "Architecture decisions (ADR candidates)", "Quality attribute scenarios", "Risks and technical debt"},
		Design:             {"Modules and components", "Data model", "API design", "Sequence flows of the key use cases", "Error handling", "Security design"},
		ImplementationPlan: {"Milestones", "Epics and tasks (table: id, task, requirement keys, depends on, estimate)", "Order and dependencies", "Definition of done per epic, with the verifying tests", "Risks to the plan"},
		FullSolution: {
			"Architecture: system context, containers and components, decisions (ADR candidates), quality attribute scenarios, risks",
			"Design: modules and components, data model, API, sequence flows, error handling, security",
			"Implementation plan: milestones, epics and tasks, order and dependencies, definition of done",
		},
	},
	tail: []string{
		"Assumptions",
		"Open questions, ambiguities and conflicts",
		"Traceability: a table with one row per requirement key, as below",
	},
}

var solSwedish = &solLang{
	lang: "sv",
	s: map[string]string{
		"title.architecture":        "Arkitektur för %[1]s",
		"title.design":              "Design av %[1]s",
		"title.implementation_plan": "Implementationsplan för %[1]s",
		"title.full_solution":       "Lösning för %[1]s: arkitektur, design och implementationsplan",
		"partTitle":                 "%[1]s — del %[2]d av %[3]d",

		"role.architecture":        "Du är en erfaren mjukvaruarkitekt. Du utformar lösningar som uppfyller kraven inom ramarna i lösningsbeskrivningen, och du gör avvägningarna tydliga.",
		"role.design":              "Du är en erfaren mjukvarudesigner och seniorutvecklare som omsätter krav och en arkitektur i en detaljerad design som kan implementeras direkt.",
		"role.implementation_plan": "Du är en erfaren teknisk projektledare som planerar implementationsarbetet så att varje uppgift kan spåras till kraven och \"klar\" betyder verifierad.",
		"role.full_solution":       "Du är en erfaren mjukvaruarkitekt och teknisk projektledare. Du tar fram en komplett lösning — arkitektur, detaljerad design och implementationsplan — spårbar till kraven.",

		"task.architecture":        "Föreslå lösningens arkitektur för kraven nedan (%[1]d). Skriv inte implementationen.",
		"task.design":              "Ta fram lösningens design för kraven nedan (%[1]d), i linje med lösningsbeskrivningen.",
		"task.implementation_plan": "Dela upp implementationen av kraven nedan (%[1]d) i milstolpar, epics och uppgifter, i en genomförbar ordning.",
		"task.full_solution":       "Ta fram arkitekturen, designen och implementationsplanen för kraven nedan (%[1]d), i ett och samma svar.",

		"h.preamble":     "Lösningsbeskrivning",
		"h.summary":      "Sammanfattning av projektet",
		"h.order":        "Beroendeordning",
		"h.general":      "För alla delar",
		"h.continuation": "Fortsättning",
		"noPreamble":     "Ingen lösningsbeskrivning angavs: ange vilka tekniker, miljöer och begränsningar du antar, och håll dem konsekventa.",
		"wholeProject":   "hela projektet",
		"scope.selected": "%[1]d valda krav",
		"scope.query":    "sökfråga",
		"scope.folder":   "mapp",
		"scope.release":  "release",
		"scope.variant":  "variant",
		"noFolder":       "Ingen mapp",
		"compactNote":    "Kompakt läge: endast titlar, kravnivåer, prioriteter och acceptanskriterier anges.",
		"orderIntro":     "Kraven med beroenden mellan sig, förutsättningar först (från relationerna \"beror på\"):",
		"cycle":          "Beroendecykel, bestäm ordningen",

		"sum.scope":       "Omfattning: %[1]s — %[2]d krav (av %[3]d i projektet)",
		"sum.part":        "Denna del: %[1]d av %[2]d, med %[3]d krav",
		"sum.obligations": "Krav per kravnivå",
		"sum.folders":     "Mappar",
		"sum.releases":    "Releaser",
		"sum.variants":    "Varianter",
		"sum.links":       "Kopplade risker: %[1]d · verifierande tester: %[2]d (av %[3]d tester i projektet)",

		"f.folder": "Mapp", "f.risks": "Risker", "f.level": "nivå", "f.mitigation": "åtgärd",
		"f.standards": "Standarder och ramverk",

		"part.architecture":        "Arkitektur",
		"part.design":              "Design",
		"part.implementation_plan": "Implementationsplan",

		"by.folder": "mapp på översta nivån",
		"by.size":   "storlek",
		"by.none":   "storlek",

		"cont.first":  "Detta är del %[1]d av %[2]d av en och samma prompt; kraven är uppdelade efter %[3]s och nästa delar följer. Besvara bara denna del. Ge element, beslut och uppgifter stabila namn och nummer: nästa delar fortsätter dem. Ge spårbarhetsraderna för denna dels krav; den samlade tabellen kommer med den sista delen.",
		"cont.middle": "Detta är del %[1]d av %[2]d. Fortsätt samma svar som i de tidigare delarna (%[3]s): återanvänd namnen och numren du införde där, upprepa inte tidigare avsnitt, lägg bara till det som dessa krav behöver, och hänvisa till befintliga element i stället för att definiera dem igen. Ge spårbarhetsraderna för denna dels krav.",
		"cont.last":   "Detta är den sista delen (%[1]d av %[2]d); de tidigare delarna omfattade %[3]s. Fortsätt som tidigare och avsluta med den samlade spårbarhetstabellen för kraven i alla delar och de fullständiga listorna över antaganden och öppna frågor.",

		"traceHeader":  "| Krav | Täcks av (element, beslut, uppgifter) | Verifierande tester | Status |",
		"traceExample": "| KEY-1 Titel | t.ex. komponent, ADR-1, endpoint, uppgift T3 | testnycklar eller \"att skriva\" | täckt / delvis / inte täckt / fråga |",
	},
	labels: map[string]string{
		"languages":          "Implementationsspråk och versioner",
		"frameworks":         "Ramverk och bibliotek",
		"environments":       "Målmiljöer",
		"architecture_style": "Arkitekturstil",
		"architecture_notes": "Arkitekturanteckningar",
		"frontend":           "Frontend (klient)",
		"backend":            "Backend (server)",
		"messaging":          "Meddelandehantering",
		"caching":            "Cachning",
		"databases":          "Databaser (beskrivna; inga inloggningsuppgifter)",
		"integrations":       "Integrationer och externa API:er",
		"deployment":         "Driftsättning och CI/CD",
		"performance":        "Prestandamål",
		"availability":       "Tillgänglighetsmål",
		"scalability":        "Skalbarhetsmål",
		"security":           "Säkerhets- och regelefterlevnadskrav",
		"conventions":        "Kodkonventioner",
		"testing":            "Teststrategi",
		"repository":         "Kodförrådets struktur",
		"out_of_scope":       "Utanför omfattningen",
		"additional":         "Ytterligare sammanhang",
	},
	styles: map[string]string{
		"monolith": "monolit", "modular_monolith": "modulär monolit", "microservices": "mikrotjänster", "serverless": "serverlös",
		"event_driven": "händelsedriven", "layered": "skiktad", "hexagonal": "hexagonal (portar och adaptrar)", "other": "annan (se anteckningarna)",
	},
	levels: map[string]string{"critical": "kritisk", "high": "hög", "medium": "medel", "low": "låg"},
	instructions: map[string][]string{
		Architecture: {
			"Beskriv systemets omgivning: användare och roller, externa system (integrationerna i lösningsbeskrivningen och de som kraven nämner) och vad som passerar systemgränsen.",
			"Beskriv containrarna (driftsättningsbara eller körbara enheter, datalager, meddelandekanaler) och de viktigaste komponenterna i var och en, med ansvar och teknikerna i lösningsbeskrivningen.",
			"Dokumentera de viktigaste arkitekturbesluten som ADR-kandidater: sammanhang, beslut, övervägda alternativ, konsekvenser och de kravnycklar som driver varje beslut.",
			"Skriv kvalitetsattributscenarier (källa, stimulus, miljö, respons, responsmått) för de icke-funktionella kraven och målen i lösningsbeskrivningen, vart och ett kopplat till kravnycklar.",
			"Lista arkitekturrisker och teknisk skuld, med de kopplade projektriskerna där de gäller, och hur arkitekturen hanterar dem.",
			"Håll dig till arkitekturstilen, målmiljöerna och det som ligger utanför omfattningen i lösningsbeskrivningen; om ett krav inte passar, säg det i stället för att ändra ramarna.",
		},
		Design: {
			"Utforma moduler och komponenter: ansvar, publika gränssnitt och beroenden, i linje med arkitekturstilen och kodförrådets struktur i lösningsbeskrivningen.",
			"Härled datamodellen från kraven: entiteter, attribut, nycklar, samband och villkor (en ER-modell).",
			"Utforma API:et: resurser eller operationer, format för anrop och svar, status- och felkoder, sidindelning, versionshantering och behörighet för varje operation.",
			"Beskriv sekvensflödena för de viktigaste användningsfallen (MUST-kraven först), steg för steg genom komponenterna.",
			"Definiera felhanteringen: validering, förväntade fel, omförsök och idempotens, tidsgränser och vad användaren eller anroparen ser.",
			"Utforma säkerheten: autentisering, behörighet, validering av indata, skydd av data i vila och under överföring, loggning för spårbarhet, samt åtgärderna för de kopplade riskerna och de mappade CWE-, ATT&CK-, D3FEND- och EMB3D-posterna.",
		},
		ImplementationPlan: {
			"Dela upp arbetet i milstolpar, epics och uppgifter; varje uppgift anger de kravnycklar den implementerar eller stödjer och är så liten att en person klarar den på några dagar.",
			"Ordna arbetet efter beroendena mellan kraven (\"beror på\", \"förfinar\"; se beroendeordningen), därefter efter kravnivå och prioritet: MUST före SHOULD före MAY.",
			"Ge varje epic en definition av klar: dess acceptanskriterier uppfyllda, dess verifierande tester (med nyckel och titel) godkända, samt de granskningar, den dokumentation och de driftsättningssteg som lösningsbeskrivningen kräver.",
			"Ange vilka tester som ska skrivas där ett krav ännu saknar verifierande test.",
			"Visa vad som kan göras parallellt och vad som blockerar annat arbete; peka ut riskerna för planen.",
		},
	},
	common: []string{
		"Spåra varje designelement (komponent, beslut, entitet, endpoint, flöde, uppgift) till de kravnycklar det tjänar, och lista de krav som inget element täcker.",
		"Hitta inte på krav eller beteenden: lyft fram oklarheter, luckor och konflikter mellan krav (även relationerna \"står i konflikt med\") som öppna frågor, med ett förslag till lösning.",
		"Lista varje antagande du gör, numrerat (A1, A2 …), och hänvisa till det där det används.",
		"Använd teknikerna, versionerna och konventionerna i lösningsbeskrivningen; föreslå ett alternativ bara med motivering.",
		"MUST-krav och deras acceptanskriterier är inte förhandlingsbara; SHOULD-krav får skjutas upp med motivering; MAY-krav är valfria.",
		"Skriv aldrig lösenord, nycklar eller andra hemligheter i svaret; hänvisa till var de förvaras.",
	},
	formats: map[string]string{
		"markdown":    "Skriv svaret i Markdown, med de numrerade avsnitten i svarsformatet som rubriker.",
		FormatMermaid: "Rita diagram som Mermaid-kodblock (```mermaid): flödes- eller komponentdiagram, sekvensdiagram för flödena, erDiagram för datamodellen.",
		FormatC4:      "Visa systemets omgivning och containrarna enligt C4-modellen (Mermaid-diagram C4Context och C4Container).",
		FormatOpenAPI: "Ge API:et som ett OpenAPI 3.1-skelett i YAML (sökvägar, operationer, scheman, felsvar, säkerhetsscheman) med kravnycklarna i x-requirements.",
		FormatSQL:     "Ge datamodellen som SQL DDL (CREATE TABLE med nycklar, villkor och index) för databasmotorn i lösningsbeskrivningen, med kravnycklarna i kommentarer.",
	},
	answers: map[string][]string{
		Architecture:       {"Systemets omgivning", "Containrar och komponenter", "Arkitekturbeslut (ADR-kandidater)", "Kvalitetsattributscenarier", "Risker och teknisk skuld"},
		Design:             {"Moduler och komponenter", "Datamodell", "API-design", "Sekvensflöden för de viktigaste användningsfallen", "Felhantering", "Säkerhetsdesign"},
		ImplementationPlan: {"Milstolpar", "Epics och uppgifter (tabell: id, uppgift, kravnycklar, beror på, uppskattning)", "Ordning och beroenden", "Definition av klar per epic, med de verifierande testerna", "Risker för planen"},
		FullSolution: {
			"Arkitektur: systemets omgivning, containrar och komponenter, beslut (ADR-kandidater), kvalitetsattributscenarier, risker",
			"Design: moduler och komponenter, datamodell, API, sekvensflöden, felhantering, säkerhet",
			"Implementationsplan: milstolpar, epics och uppgifter, ordning och beroenden, definition av klar",
		},
	},
	tail: []string{
		"Antaganden",
		"Öppna frågor, oklarheter och konflikter",
		"Spårbarhet: en tabell med en rad per kravnyckel, enligt nedan",
	},
}
