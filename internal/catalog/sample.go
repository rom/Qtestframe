package catalog

import (
	"fmt"
	"strings"
	"testing/fstest"
)

// SampleFS generates a catalogue directory "data" with groups.yaml and
// areas areas of n requirements and n tests each (requirement i is
// verified by test i, the first requirement of an area also by the second
// test). Every item passes the checks of Load. It is meant for tests and
// for trying the browser and the import at scale; the built-in content is
// data/*.yaml.
func SampleFS(areas, n int) fstest.MapFS {
	groups := []string{"security", "privacy", "quality", "operations", "usability", "functional"}
	var g strings.Builder
	g.WriteString("groups:\n")
	for _, id := range groups {
		fmt.Fprintf(&g, "  - id: %s\n    name: {en: Group %s, sv: Grupp %s}\n", id, id, id)
	}
	fsys := fstest.MapFS{"data/groups.yaml": {Data: []byte(g.String())}}
	obligations := []struct{ ob, en, sv string }{{"must", "MUST", "SKA"}, {"should", "SHOULD", "BÖR"}, {"may", "MAY", "FÅR"}}
	priorities := []string{"critical", "high", "medium", "low"}
	types := []string{"security", "performance", "functional", "usability"}
	for a := 0; a < areas; a++ {
		prefix := fmt.Sprintf("S%c%c", 'A'+a/26%26, 'A'+a%26)
		var b strings.Builder
		fmt.Fprintf(&b, "area: sample-%d\nprefix: %s\ngroup: %s\n", a+1, prefix, groups[a%len(groups)])
		fmt.Fprintf(&b, "name: {en: Sample area %d, sv: Exempelområde %d}\n", a+1, a+1)
		fmt.Fprintf(&b, "description: {en: Generated sample items %d., sv: Genererade exempel %d.}\n", a+1, a+1)
		b.WriteString("requirements:\n")
		for i := 1; i <= n; i++ {
			o := obligations[i%len(obligations)]
			tests := fmt.Sprintf("%s-T%02d", prefix, i)
			if i == 1 && n > 1 {
				tests += fmt.Sprintf(", %s-T02", prefix)
			}
			fmt.Fprintf(&b, `  - id: %[1]s-%02[2]d
    type: %[3]s
    obligation: %[4]s
    priority: %[5]s
    tags: [sample, tag-%[6]d]
    refs: ["ISO/IEC 25010 §%[2]d"]
    tests: [%[7]s]
    en:
      title: Audit record %[2]d of area %[8]d
      description: The system %[9]s write audit record %[2]d within 2 seconds of the event.
      rationale: Records support the investigation of incidents.
      criteria:
        - method: test
          text: Record %[2]d is written within 2 seconds.
          threshold: "≤ 2 s"
    sv:
      title: Granskningspost %[2]d i område %[8]d
      description: Systemet %[10]s skriva granskningspost %[2]d inom 2 sekunder efter händelsen.
      rationale: Posterna stödjer utredning av incidenter.
      criteria:
        - method: test
          text: Post %[2]d skrivs inom 2 sekunder.
          threshold: "≤ 2 s"
`, prefix, i, types[i%len(types)], o.ob, priorities[i%len(priorities)], i%5, tests, a+1, o.en, o.sv)
		}
		b.WriteString("tests:\n")
		for i := 1; i <= n; i++ {
			exec := "manual"
			if i%2 == 0 {
				exec = "automated"
			}
			fmt.Fprintf(&b, `  - id: %[1]s-T%02[2]d
    priority: %[3]s
    execution: %[4]s
    tags: [sample]
    refs: []
    en:
      title: Audit record %[2]d is written
      description: Checks audit record %[2]d.
      preconditions: An administrator is signed in.
      expected: The record is written.
      steps:
        - action: Trigger event %[2]d.
          expected: Record %[2]d appears in the audit log.
          data: event-%[2]d
    sv:
      title: Granskningspost %[2]d skrivs
      description: Kontrollerar granskningspost %[2]d.
      preconditions: En administratör är inloggad.
      expected: Posten skrivs.
      steps:
        - action: Utlös händelse %[2]d.
          expected: Post %[2]d syns i granskningsloggen.
          data: event-%[2]d
`, prefix, i, priorities[i%len(priorities)], exec)
		}
		fsys[fmt.Sprintf("data/sample-%03d.yaml", a+1)] = &fstest.MapFile{Data: []byte(b.String())}
	}
	return fsys
}
