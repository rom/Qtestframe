package report

import (
	"strings"

	"github.com/rom/qtestframe/internal/export"
	"github.com/rom/qtestframe/internal/model"
)

// EnumEnvironmentType names the types of test environments.
const EnumEnvironmentType = "environment_type"

func init() {
	for k, v := range environmentTexts {
		if _, ok := texts[Swedish][k]; !ok {
			texts[Swedish][k] = v
		}
	}
	enums[English][EnumEnvironmentType] = map[string]string{"dev": "Development", "test": "Test", "staging": "Staging", "pre_prod": "Pre-production",
		"prod_like": "Production-like", "lab": "Lab", "hil_rig": "HIL rig", "device": "Device"}
	enums[Swedish][EnumEnvironmentType] = map[string]string{"dev": "Utveckling", "test": "Test", "staging": "Staging", "pre_prod": "Förproduktion",
		"prod_like": "Produktionslik", "lab": "Labb", "hil_rig": "HIL-rigg", "device": "Enhet"}
}

var environmentTexts = map[string]string{
	"Tested environments":                  "Testade miljöer",
	"Components and versions at execution": "Komponenter och versioner vid exekveringen",
	"Last result":                          "Senaste resultat",
	"Environments tested":                  "Testade miljöer",
	"No result of the release's runs names an environment.": "Inget resultat från releasens körningar anger någon miljö.",
}

// componentsText shows the components of an environment: "app 2.3.1, DB 16".
func componentsText(cs []model.Component) string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, strings.TrimSpace(c.Name+" "+c.Version))
	}
	return strings.Join(out, ", ")
}

// environmentSection lists the environments (and component versions) the
// results of a release's runs were obtained in.
func (l loc) environmentSection(list []model.ReleaseEnvironment) export.Section {
	sec := export.Section{Name: "environments", Title: l.t("Tested environments"),
		Columns: l.cols("Environment", "Type", "Components and versions at execution", "Executions", "Passed", "Failed", "Blocked", "Last result")}
	for _, e := range list {
		sec.Rows = append(sec.Rows, []string{e.Name, l.enum(EnumEnvironmentType, e.Type), componentsText(e.Components), itoa(e.Executions),
			itoa(e.Passed), itoa(e.Failed), itoa(e.Blocked), tsPtr(e.LastAt)})
	}
	if len(sec.Rows) == 0 {
		sec.Rows = append(sec.Rows, []string{l.t("No result of the release's runs names an environment."), "", "", "", "", "", "", ""})
	}
	return sec
}
