package similarity

import "strings"

// The word lists of the scorer: stop words, synonyms and abbreviations of
// the domain (mapped to language-neutral concepts, so that "log in",
// "sign in" and "logga in" match), and groups of mutually exclusive
// values for the conflict detection.

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// stopWords are ignored (both languages: a requirement may mix them).
// Obligation keywords are stop words: the obligation is compared by the
// conflict detection, not by the similarity.
var stopWords = set(`
a an the and or but if then else of to in on at by for from with without into onto over under as is are was were be been being
it its this that these those there here which who whom whose what when where how why all any each every some such no nor not only
own same so than too very able can could will would should shall must may might do does did done have has had having
i we you he she they them their our your his her my me us also via per e g eg i e ie etc after before then
system systemet application applikationen
och eller men om så att i på av för från med utan till över under som är var vara varit blir bli blev
det den de detta denna dessa där här vilken vilket vilka vem vad när var hur varför alla varje någon något några
ingen inget inga inte icke endast bara också även än mycket kan kunna ska skall måste bör borde får fick få har hade
ha en ett man sig sin sitt sina dess deras vi ni han hon de dem vår er efter innan före sedan
`)

// concepts map phrases (one or more words, lower case) to a concept. A
// concept is a token of its own ("#login") and replaces the phrase.
var concepts = map[string][]string{
	"login":    {"log in", "log on", "logon", "login", "logins", "sign in", "signin", "sign on", "signon", "logga in", "logga på", "inloggning", "inloggningen", "inloggningar", "loggar in", "loggat in", "logged in", "logging in", "logs in", "signed in", "signing in", "signs in", "authenticate", "authenticates", "authenticated", "authentication", "autentisera", "autentiserar", "autentiseras", "autentisering"},
	"logout":   {"log out", "log off", "logout", "logoff", "sign out", "signout", "logga ut", "utloggning", "utloggningen", "loggar ut", "logged out", "logging out", "logs out", "signed out", "signs out"},
	"signup":   {"sign up", "signup", "register", "registers", "registered", "registration", "enrol", "enroll", "enrolment", "enrollment", "registrera", "registrerar", "registrering", "registreringen", "skapa konto"},
	"user":     {"user", "users", "end user", "end users", "enduser", "användare", "användaren", "användarna", "användares", "brukare", "brukaren", "brukarna", "slutanvändare", "slutanvändaren"},
	"password": {"password", "passwords", "passphrase", "passphrases", "pwd", "passcode", "lösenord", "lösenordet", "lösenorden", "lösen", "passerord", "credential", "credentials", "inloggningsuppgifter"},
	"email":    {"email", "emails", "e mail", "e mails", "mail address", "email address", "e mail address", "e post", "epost", "e postadress", "e postadressen", "epostadress", "mejl", "mejladress", "mail", "mails"},
	"mfa":      {"mfa", "2fa", "two factor", "two factor authentication", "multi factor", "multifactor", "multi factor authentication", "tvåfaktor", "tvåfaktorsautentisering", "flerfaktorsautentisering", "tvåstegsverifiering"},
	"delete":   {"delete", "deletes", "deleted", "deleting", "deletion", "remove", "removes", "removed", "removing", "removal", "erase", "erases", "erased", "radera", "raderar", "raderas", "raderad", "raderade", "ta bort", "tar bort", "tas bort", "borttagning", "borttagen"},
	"show":     {"show", "shows", "shown", "showing", "display", "displays", "displayed", "displaying", "present", "presents", "presented", "visa", "visar", "visas", "visad", "visade", "presentera", "presenterar", "presenteras"},
	"admin":    {"admin", "admins", "administrator", "administrators", "sysadmin", "administratör", "administratören", "administratörer", "systemadministratör"},
	"account":  {"account", "accounts", "user account", "user accounts", "konto", "kontot", "konton", "användarkonto", "användarkontot"},
	"database": {"db", "database", "databases", "databas", "databasen"},
	"ui":       {"ui", "gui", "user interface", "graphical user interface", "användargränssnitt", "användargränssnittet", "gränssnitt", "gränssnittet"},
	"response": {"response time", "response times", "svarstid", "svarstiden", "svarstider", "responstid", "responstiden"},
	"lock":     {"lock", "locks", "locked", "locking", "lockout", "lock out", "locked out", "lås", "låsa", "låser", "låses", "låst", "spärr", "spärren", "spärra", "spärrar", "spärras", "spärrad", "spärrat"},
	"attempt":  {"attempt", "attempts", "try", "tries", "försök", "försöket", "försöken"},
	"timeout":  {"timeout", "time out", "timed out", "session timeout", "inactivity timeout", "tidsgräns", "tidsgränsen", "timeout"},
	"report":   {"report", "reports", "rapport", "rapporten", "rapporter"},
	"export":   {"export", "exports", "exported", "exporting", "exportera", "exporterar", "exporteras", "export"},
	"encrypt":  {"encrypt", "encrypts", "encrypted", "encryption", "kryptera", "krypterar", "krypteras", "krypterad", "krypterade", "kryptering"},
	"backup":   {"backup", "backups", "back up", "säkerhetskopia", "säkerhetskopior", "säkerhetskopiera", "säkerhetskopieras", "backup"},
}

// conceptPhrases: the phrases of concepts as word lists, longest first.
type conceptPhrase struct {
	words   []string
	concept string
}

var conceptIndex = func() map[string][]conceptPhrase {
	idx := map[string][]conceptPhrase{}
	for c, phrases := range concepts {
		for _, p := range phrases {
			ws := strings.Fields(p)
			idx[ws[0]] = append(idx[ws[0]], conceptPhrase{words: ws, concept: c})
		}
	}
	for k, ps := range idx {
		// longest first
		for i := 1; i < len(ps); i++ {
			for j := i; j > 0 && len(ps[j].words) > len(ps[j-1].words); j-- {
				ps[j], ps[j-1] = ps[j-1], ps[j]
			}
		}
		idx[k] = ps
	}
	return idx
}()

// exclusiveGroups are sets of values of which a thing has only one: two
// requirements about the same thing that name different values of a group
// contradict each other. Each value lists its words (any language).
var exclusiveGroups = [][][]string{
	{{"enabled", "activated", "aktiverad", "aktiverat", "aktiverade", "påslagen", "påslaget", "aktiv"}, {"disabled", "deactivated", "inaktiverad", "inaktiverat", "inaktiverade", "avstängd", "avstängt", "avslagen", "avaktiverad", "inaktiv"}},
	{{"mandatory", "required", "compulsory", "obligatory", "obligatorisk", "obligatoriskt", "obligatoriska"}, {"optional", "voluntary", "frivillig", "frivilligt", "frivilliga", "valfri", "valfritt", "valfria"}},
	{{"allowed", "permitted", "tillåten", "tillåtet", "tillåtna"}, {"forbidden", "prohibited", "disallowed", "förbjuden", "förbjudet", "förbjudna", "otillåten", "otillåtet"}},
	{{"encrypted", "krypterad", "krypterat", "krypterade"}, {"unencrypted", "plaintext", "cleartext", "okrypterad", "okrypterat", "okrypterade", "klartext"}},
	{{"public", "publik", "publikt", "offentlig", "offentligt", "offentliga"}, {"private", "privat", "privata"}},
	{{"synchronous", "synchronously", "synkron", "synkront"}, {"asynchronous", "asynchronously", "asynkron", "asynkront"}},
	{{"automatic", "automatically", "automatisk", "automatiskt", "automatiska"}, {"manual", "manually", "manuell", "manuellt", "manuella"}},
	{{"visible", "synlig", "synligt", "synliga"}, {"hidden", "invisible", "dold", "dolt", "dolda", "osynlig", "osynligt"}},
	{{"always", "alltid"}, {"never", "aldrig"}},
	{{"online", "uppkopplad"}, {"offline", "nedkopplad", "frånkopplad"}},
	{{"readonly", "read only", "skrivskyddad", "skrivskyddat", "läsbehörighet"}, {"editable", "writable", "redigerbar", "redigerbart", "ändringsbar"}},
	{{"stateless", "tillståndslös"}, {"stateful", "tillståndsbaserad"}},
	{{"centralised", "centralized", "centraliserad"}, {"decentralised", "decentralized", "distributed", "decentraliserad", "distribuerad"}},
}

type exclusiveValue struct {
	group, value int
}

var exclusiveIndex = func() map[string]exclusiveValue {
	m := map[string]exclusiveValue{}
	for g, values := range exclusiveGroups {
		for v, words := range values {
			for _, w := range words {
				m[w] = exclusiveValue{g, v}
			}
		}
	}
	return m
}()

// numberWords are written numbers ("five", "fem"); one, en and ett are
// left out (articles).
var numberWords = map[string]string{
	"two": "2", "three": "3", "four": "4", "five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10",
	"eleven": "11", "twelve": "12", "fifteen": "15", "twenty": "20", "thirty": "30",
	"två": "2", "tre": "3", "fyra": "4", "fem": "5", "sex": "6", "sju": "7", "åtta": "8", "nio": "9", "tio": "10",
	"elva": "11", "tolv": "12", "femton": "15", "tjugo": "20", "trettio": "30",
}

// negations reverse the polarity of a statement without an obligation
// keyword ("is not stored", "lagras inte").
var negations = set(`not no never cannot can't won't don't doesn't isn't aren't inte ej icke aldrig ingen inget inga`)
