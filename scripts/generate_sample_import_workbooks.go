package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/xuri/excelize/v2"
)

type property struct {
	Name    string
	Address string
	City    string
	Prefix  string
	Units   int
	RentKES int
	DueDay  int
	Paybill string
	Bank    string
	Account string
}

func main() {
	outDir := filepath.Join("sample-data", "imports")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	properties := []property{
		{Name: "Riverside Heights", Address: "Riverside Drive", City: "Nairobi", Prefix: "RH", Units: 56, RentKES: 58000, DueDay: 5, Paybill: "522522", Bank: "KCB Bank", Account: "1102459001"},
		{Name: "Kilimani Gardens", Address: "Kindaruma Road", City: "Nairobi", Prefix: "KG", Units: 54, RentKES: 47000, DueDay: 3, Paybill: "400200", Bank: "Equity Bank", Account: "0760291842331"},
		{Name: "Mombasa Road Lofts", Address: "Mombasa Road", City: "Nairobi", Prefix: "MRL", Units: 52, RentKES: 42000, DueDay: 7, Paybill: "247247", Bank: "NCBA Bank", Account: "8800134029"},
	}

	var masterRows [][]any
	for _, property := range properties {
		rows := rowsForProperty(property)
		masterRows = append(masterRows, rows...)
		if err := writeWorkbook(filepath.Join(outDir, safeName(property.Name)+".xlsx"), property.Name, rows); err != nil {
			log.Fatal(err)
		}
	}
	if err := writeWorkbook(filepath.Join(outDir, "agency-master-import.xlsx"), "Master Import", masterRows); err != nil {
		log.Fatal(err)
	}
}

func rowsForProperty(p property) [][]any {
	firstNames := []string{"Amina", "Brian", "Caroline", "David", "Esther", "Felix", "Grace", "Hassan", "Irene", "James", "Karen", "Leon", "Mercy", "Noah", "Olivia", "Peter", "Queen", "Ray", "Stella", "Tom", "Uma", "Victor", "Winnie", "Yusuf", "Zara"}
	lastNames := []string{"Mwangi", "Otieno", "Wanjiku", "Kariuki", "Achieng", "Mutiso", "Njeri", "Omondi", "Chebet", "Mweni", "Maina", "Kimani", "Adhiambo", "Kiptoo", "Wambui"}
	rows := make([][]any, 0, p.Units)
	for i := 1; i <= p.Units; i++ {
		name := fmt.Sprintf("%s %s", firstNames[(i-1)%len(firstNames)], lastNames[(i-1)%len(lastNames)])
		unit := fmt.Sprintf("%s-%03d", p.Prefix, i)
		phone := fmt.Sprintf("+2547%08d", 10000000+i)
		email := fmt.Sprintf("%s.%s.%s%03d@example.com", lower(firstNames[(i-1)%len(firstNames)]), lower(lastNames[(i-1)%len(lastNames)]), lower(p.Prefix), i)
		nationalID := fmt.Sprintf("29%06d", 300000+i)
		rent := p.RentKES + (i%5)*1500
		deposit := rent
		paymentMethod := "mpesa_paybill"
		bankName := ""
		bankAccount := ""
		mpesaPaybill := p.Paybill
		mpesaAccount := unit
		if i%3 == 0 {
			paymentMethod = "bank"
			bankName = p.Bank
			bankAccount = fmt.Sprintf("%s-%03d", p.Account, i)
			mpesaPaybill = ""
			mpesaAccount = ""
		}
		rows = append(rows, []any{name, phone, email, nationalID, p.Name, p.Address, p.City, unit, rent, "2026-05-01", p.DueDay, deposit, paymentMethod, bankName, bankAccount, mpesaPaybill, mpesaAccount})
	}
	return rows
}

func writeWorkbook(path, sheet string, rows [][]any) error {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", sheet)
	headers := []any{"full_name", "phone", "email", "national_id", "property_name", "property_address", "city", "unit_label", "monthly_rent_kes", "lease_start_date", "due_day", "deposit_kes", "payment_method", "bank_name", "bank_account_number", "mpesa_paybill", "mpesa_account_number"}
	if err := f.SetSheetRow(sheet, "A1", &headers); err != nil {
		return err
	}
	for i, row := range rows {
		if err := f.SetSheetRow(sheet, fmt.Sprintf("A%d", i+2), &row); err != nil {
			return err
		}
	}
	for i := 1; i <= len(headers); i++ {
		col, _ := excelize.ColumnNumberToName(i)
		_ = f.SetColWidth(sheet, col, col, 20)
	}
	return f.SaveAs(path)
}

func safeName(value string) string {
	out := ""
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out += string(r)
		} else {
			out += "-"
		}
	}
	return out
}

func lower(value string) string {
	out := ""
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out += string(r)
	}
	return out
}
