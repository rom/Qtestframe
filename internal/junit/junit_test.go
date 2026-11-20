package junit

import (
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

const playwright = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites id="" name="" tests="4" failures="1" skipped="1" errors="0" time="5.3">
<testsuite name="checkout.spec.ts" timestamp="2026-11-20T10:00:00.000Z" hostname="chromium" tests="4" failures="1" skipped="1" time="4.1" errors="0">
<testcase name="checkout › pays with card @SHOP-TC-12" classname="checkout.spec.ts" time="1.25">
</testcase>
<testcase name="checkout › refunds" classname="checkout.spec.ts" time="2.5">
<properties>
<property name="qtestframe" value="SHOP-TC-13">
</property>
</properties>
<failure message="checkout.spec.ts:20:5 refunds" type="FAILURE">
<![CDATA[  Error: expect(received).toBe(expected)
    Expected: 2
    Received: 3
]]>
</failure>
<system-out>
<![CDATA[[[ATTACHMENT|test-results/refund/screenshot.png]]
]]>
</system-out>
</testcase>
<testcase name="checkout › later" classname="checkout.spec.ts" time="0">
<skipped>
</skipped>
</testcase>
<testcase name="checkout › no key" classname="checkout.spec.ts" time="0.1">
</testcase>
</testsuite>
</testsuites>`

const pytest = `<?xml version="1.0" encoding="utf-8"?><testsuites><testsuite name="pytest" errors="1" failures="1" skipped="1" tests="5" time="0.512" timestamp="2026-11-20T10:00:00" hostname="ci"><testcase classname="tests.test_cart.TestCart" name="test_add_item" time="0.010"><properties><property name="test_key" value="shop-tc-3" /></properties></testcase><testcase classname="tests.test_cart.TestCart" name="test_login[SHOP-TC-4]" time="0.020"><failure message="AssertionError: assert 1 == 2">def test_login():
&gt;       assert 1 == 2
E       AssertionError</failure></testcase><testcase classname="tests.test_cart" name="test_SHOP_TC_5_remove" time="0.003"><error message="failed on setup with &quot;fixture 'db' not found&quot;">file tests/test_cart.py, line 9</error></testcase><testcase classname="tests.test_cart" name="test_skip" time="0.000"><skipped type="pytest.skip" message="not on CI">tests/test_cart.py:12: not on CI</skipped></testcase><testcase classname="tests.test_cart" name="test_out" time="0.001"><system-out>hello</system-out><system-err>warn</system-err></testcase></testsuite></testsuites>`

// go-junit-report v2
const goReport = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="3" failures="1" skipped="1">
	<testsuite name="github.com/acme/shop/cart" tests="3" failures="1" errors="0" id="0" hostname="ci" skipped="1" time="0.012" timestamp="2026-11-20T10:00:00Z">
		<properties>
			<property name="go.version" value="go1.26.0 linux/amd64"></property>
		</properties>
		<testcase name="TestCart/SHOP-TC-7_add" classname="github.com/acme/shop/cart" time="0.001"></testcase>
		<testcase name="TestCart/SHOP-TC-7_remove" classname="github.com/acme/shop/cart" time="0.002">
			<failure message="Failed"><![CDATA[    cart_test.go:21: got 1, want 2]]></failure>
		</testcase>
		<testcase name="TestSlow" classname="github.com/acme/shop/cart" time="0.000">
			<skipped message="Skipped"><![CDATA[    cart_test.go:30: short mode]]></skipped>
		</testcase>
	</testsuite>
</testsuites>`

// Maven Surefire: a single <testsuite> root, keys in the class name.
const surefire = `<?xml version="1.0" encoding="ISO-8859-1"?>
<testsuite xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" name="com.acme.ShopTC8Test" time="1,234.5" tests="2" errors="0" skipped="0" failures="0">
  <properties><property name="java.version" value="21"/></properties>
  <testcase name="checkout" classname="com.acme.SHOP_TC_8_CheckoutTest" time="1,000.25"/>
  <testcase name="caf` + "\xe9" + `" classname="com.acme.Other" time="0.25"><system-out>caf` + "\xe9" + `</system-out></testcase>
</testsuite>`

func parse(t *testing.T, s string) []Case {
	t.Helper()
	cs, err := ParseBytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func TestPlaywright(t *testing.T) {
	cs := parse(t, playwright)
	if len(cs) != 4 {
		t.Fatalf("%d cases", len(cs))
	}
	if cs[0].Key() != "SHOP-TC-12" || cs[0].Status != Passed || cs[0].DurationMS() != 1250 || cs[0].Suite != "checkout.spec.ts" {
		t.Errorf("case 0: %+v", cs[0])
	}
	if cs[1].Key() != "SHOP-TC-13" || cs[1].Status != Failed || !strings.Contains(cs[1].Output, "Expected: 2") ||
		!strings.Contains(cs[1].Output, "[failure]\nFAILURE: checkout.spec.ts:20:5 refunds") || !strings.Contains(cs[1].Output, "[system-out]\n[[ATTACHMENT") {
		t.Errorf("case 1: %+v\n%s", cs[1], cs[1].Output)
	}
	if cs[2].Status != Skipped || cs[2].Output != "" || cs[3].Key() != "" {
		t.Errorf("cases 2/3: %+v %+v", cs[2], cs[3])
	}
	m, u := Aggregate(cs)
	if len(m) != 2 || len(u) != 2 || u[0].Name != "checkout › later" || u[0].Status != Skipped || u[1].Key != "" {
		t.Errorf("aggregate: %+v / %+v", m, u)
	}
}

func TestPytest(t *testing.T) {
	cs := parse(t, pytest)
	want := []struct{ key, status string }{
		{"SHOP-TC-3", Passed}, {"SHOP-TC-4", Failed}, {"SHOP-TC-5", Failed}, {"", Skipped}, {"", Passed},
	}
	for i, w := range want {
		if cs[i].Key() != w.key || cs[i].Status != w.status {
			t.Errorf("case %d: key %q status %q, want %+v", i, cs[i].Key(), cs[i].Status, w)
		}
	}
	if !strings.Contains(cs[1].Output, ">       assert 1 == 2") || !strings.Contains(cs[2].Output, `[error]`) ||
		!strings.Contains(cs[2].Output, `fixture 'db' not found`) || !strings.Contains(cs[3].Output, "pytest.skip: not on CI") {
		t.Errorf("outputs: %q %q %q", cs[1].Output, cs[2].Output, cs[3].Output)
	}
	if cs[4].Output != "[system-out]\nhello\n\n[system-err]\nwarn" {
		t.Errorf("out: %q", cs[4].Output)
	}
	if cs[0].DurationMS() != 10 {
		t.Error(cs[0].DurationMS())
	}
}

func TestGoJUnitReport(t *testing.T) {
	cs := parse(t, goReport)
	m, u := Aggregate(cs)
	if len(m) != 1 || len(u) != 1 {
		t.Fatalf("%+v %+v", m, u)
	}
	r := m[0]
	if r.Key != "SHOP-TC-7" || r.Status != Failed || r.DurationMS != 3 || r.Cases != 2 ||
		!strings.Contains(r.Output, "== TestCart/SHOP-TC-7_remove\n[failure]") {
		t.Errorf("%+v", r)
	}
	if u[0].Name != "TestSlow" || u[0].Status != Skipped {
		t.Errorf("%+v", u[0])
	}
}

func TestSurefire(t *testing.T) {
	cs := parse(t, surefire)
	if len(cs) != 2 || cs[0].Key() != "SHOP-TC-8" || cs[0].DurationMS() != 1000250 || cs[0].Suite != "com.acme.ShopTC8Test" {
		t.Fatalf("%+v", cs)
	}
	if cs[1].Name != "café" || cs[1].Output != "[system-out]\ncafé" {
		t.Errorf("latin1: %q %q", cs[1].Name, cs[1].Output)
	}
}

func TestNestedSuitesAndCombine(t *testing.T) {
	doc := `<testsuites><testsuite name="outer"><testsuite name="inner">
	<testcase name="a SHOP-TC-1" time="1"/>
	<testcase name="b SHOP-TC-1" time="2"><failure>boom</failure></testcase>
	<testcase name="c SHOP-TC-1" time="x"/>
	<testcase name="d SHOP-TC-2" time="-1"><skipped/></testcase>
	<testcase name="e SHOP-TC-2" time="0.5"/>
	<testcase name="same" time="1"><system-out>one</system-out></testcase>
	<testcase name="same" time="1"><failure message="m"/></testcase>
	<testcase classname="only.Class" time="1"><properties><property name="other" value="X"/><property name="QTESTFRAME.KEY">SHOP-TC-9</property></properties></testcase>
	<testcase name="blank prop SHOP-TC-10"><properties><property name="test_key" value=" "/></properties><failure/></testcase>
	</testsuite></testsuite></testsuites>`
	cs := parse(t, doc)
	if cs[0].Suite != "inner" {
		t.Error(cs[0].Suite)
	}
	m, u := Aggregate(cs)
	if len(m) != 4 || len(u) != 1 {
		t.Fatalf("%+v / %+v", m, u)
	}
	if m[0].Key != "SHOP-TC-1" || m[0].Status != Failed || m[0].DurationMS != 3000 || m[0].Cases != 3 || !strings.HasPrefix(m[0].Output, "== b SHOP-TC-1\n[failure]\nboom") {
		t.Errorf("tc1: %+v", m[0])
	}
	if m[1].Key != "SHOP-TC-2" || m[1].Status != Passed || m[1].DurationMS != 500 {
		t.Errorf("tc2: %+v", m[1])
	}
	if m[2].Key != "SHOP-TC-9" || m[2].Name != "only.Class" {
		t.Errorf("tc9: %+v", m[2])
	}
	// an empty <failure/> still fails; a blank key property falls back to the name
	if m[3].Key != "SHOP-TC-10" || m[3].Status != Failed || m[3].Output != "" {
		t.Errorf("tc10: %+v", m[3])
	}
	if u[0].Name != "same" || u[0].Status != Failed || u[0].Cases != 2 || u[0].Output != "== same\n[system-out]\none\n\n== same\n[failure]\nm" {
		t.Errorf("same: %+v %q", u[0], u[0].Output)
	}
}

func TestMalformed(t *testing.T) {
	for name, doc := range map[string]string{
		"empty":      "",
		"blank":      "  \n",
		"not xml":    "{\"results\":[]}",
		"truncated":  "<testsuites><testsuite><testcase name='a'>",
		"wrong root": `<html><body/></html>`,
		"two roots":  `<testsuite/><testsuite/>`,
		"trailing":   `<testsuite/><`,
		"encoding":   `<?xml version="1.0" encoding="EBCDIC"?><testsuite/>`,
	} {
		if _, err := ParseBytes([]byte(doc)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Parse(iotest.ErrReader(errors.New("io"))); !errors.Is(err, ErrInvalid) {
		t.Error(err)
	}
	if _, err := Parse(iotest.TimeoutReader(strings.NewReader(`<?xml version="1.0" encoding="latin1"?><testsuite/>`))); err == nil {
		t.Error("expected read error")
	}
	cs, err := ParseBytes([]byte(`<!-- c --><testsuites/><!-- trailing comment -->` + "\n"))
	if err != nil || len(cs) != 0 {
		t.Error(cs, err)
	}
}

func TestTruncate(t *testing.T) {
	if Truncate("abc", 5) != "abc" {
		t.Error()
	}
	s := Truncate(strings.Repeat("é", 100), 50)
	if len(s) > 50 || !strings.HasSuffix(s, "(truncated)") || !strings.HasPrefix(s, "é") {
		t.Errorf("%q", s)
	}
	if s := Truncate("abcdef", 2); s != "\n… (truncated)" {
		t.Errorf("%q", s)
	}
	long := `<testsuite><testcase name="SHOP-TC-1"><system-out>` + strings.Repeat("x", MaxOutput+10) + `</system-out></testcase></testsuite>`
	cs := parse(t, long)
	if len(cs[0].Output) > MaxOutput {
		t.Error(len(cs[0].Output))
	}
}
