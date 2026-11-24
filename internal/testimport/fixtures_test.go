package testimport

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// qtestXLSX builds the qTest workbook from the CSV sample (the same cases).
func qtestXLSX(t *testing.T) []byte {
	t.Helper()
	tb, err := readCSV(fixture(t, "qtest-cases.csv"))
	if err != nil {
		t.Fatal(err)
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Test Cases"
	_ = f.SetSheetName("Sheet1", sheet)
	_ = f.SetCellValue(sheet, "A1", "Exported from qTest Manager (synthetic sample)")
	write := func(r int, vals []string) {
		for c, v := range vals {
			name, _ := excelize.CoordinatesToCellName(c+1, r)
			_ = f.SetCellValue(sheet, name, v)
		}
	}
	write(3, tb.headers)
	for i, row := range tb.rows {
		write(i+4, row)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestWriteQTestFixture regenerates testdata/qtest-cases.xlsx with
// QTF_WRITE_FIXTURES=1.
func TestWriteQTestFixture(t *testing.T) {
	if os.Getenv("QTF_WRITE_FIXTURES") == "" {
		t.Skip("set QTF_WRITE_FIXTURES=1 to regenerate the qTest workbook")
	}
	if err := os.WriteFile(filepath.Join("testdata", "qtest-cases.xlsx"), qtestXLSX(t), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
