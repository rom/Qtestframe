package app

import (
	"context"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/prompt"
	"github.com/rom/qtestframe/internal/store"
)

// seedSolution makes the demo project a software project with a technical
// context and a filled solution preamble (from the Go + PostgreSQL + Vue
// template, adapted to the banking portal), so AI prompts offers the
// solution prompts (architecture, design, implementation plan, full
// solution). Two saves give the preamble a history.
func seedSolution(ctx context.Context, st *store.Store, tenantID string, p *model.Project) error {
	p.Type = model.ProjectTypeSoftware
	p.TechContext = "Go backend with a REST API, Vue 3 single-page application, PostgreSQL. See the solution preamble for the full stack."
	if err := st.UpdateProject(ctx, tenantID, p); err != nil {
		return err
	}
	pre := prompt.FindTemplate("go-postgres-vue").Preamble
	pre.Template = "go-postgres-vue"
	if err := st.SaveSolutionPreamble(ctx, tenantID, p.ID, &pre, "seed"); err != nil {
		return err
	}
	pre.Environments = "Linux containers on the bank's private Kubernetes platform (two data centres, active/passive); customers on current evergreen browsers on desktop and mobile; staff on managed Windows laptops with Edge"
	pre.Integrations = "Core banking system (SOAP/REST through the integration layer), card processor (ISO 8583 gateway), BankID for strong customer authentication, SMS and e-mail notification services, Open Banking (PSD2) API for third-party providers"
	pre.Messaging = "Kafka for account and payment events from the core banking system; outbox pattern for events the portal publishes"
	pre.Caching = "Redis for sessions and rate limiting; no caching of account balances"
	pre.Databases = "PostgreSQL 16 owned by the portal (customers' settings, payees, audit trail); balances and transactions stay in the core banking system and are read through its API; versioned SQL migrations embedded in the binary; the database user and password come from the platform's secret store"
	pre.Performance = "Log-in under 2 s and account overview under 1 s at the 95th percentile with 2 000 concurrent customers"
	pre.Availability = "99.95 % outside the weekly maintenance window; payments queue when the core banking system is down"
	pre.Security = "Strong customer authentication (PSD2 SCA) with BankID or one-time codes; sessions time out after 10 minutes of inactivity; OWASP ASVS level 3 for payment functions; personal data per GDPR; audit trail of every payment and change of payee; PCI DSS scope kept out of the portal"
	pre.OutOfScope = "The core banking system itself; the branch office applications; native mobile apps (a separate project)"
	pre.Additional = "The portal replaces an older Java portal; customers' saved payees must be migrated. Regulator: Finansinspektionen."
	return st.SaveSolutionPreamble(ctx, tenantID, p.ID, &pre, "seed")
}
