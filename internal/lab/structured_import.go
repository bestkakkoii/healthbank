// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// Portable reports contain only original laboratory fields. They are not identity
// proof, and neither serialized grades nor derived calculations are trusted.
const portableSchema = "healthbank.lab.v1"

var portableWarning = []string{"匯入檔案不代表官方完整歷史；僅重新整理檔案內的原始檢驗欄位，不驗證檔案所屬身分。"}
var portableFields = map[string]bool{
	"check_date": true, "order_code": true, "order_name": true, "item_name": true,
	"result": true, "unit": true, "reference": true, "institution": true,
	"visit_date": true, "source": true, "official_flag": true,
}

type portableRow struct {
	CheckDate    string `json:"check_date"`
	OrderCode    string `json:"order_code"`
	OrderName    string `json:"order_name"`
	ItemName     string `json:"item_name"`
	Result       string `json:"result"`
	Unit         string `json:"unit"`
	Reference    string `json:"reference"`
	Institution  string `json:"institution"`
	VisitDate    string `json:"visit_date"`
	Source       string `json:"source"`
	OfficialFlag string `json:"official_flag"`
}

func (v portableRow) raw() RawRecord {
	return RawRecord{CheckDate: v.CheckDate, OrderCode: v.OrderCode, OrderName: v.OrderName, ItemName: v.ItemName, Result: v.Result, Unit: v.Unit, Reference: v.Reference, Institution: v.Institution, VisitDate: v.VisitDate, Source: v.Source, OfficialFlag: v.OfficialFlag}
}
func validatePortable(rows []portableRow) ([]RawRecord, []string, error) {
	if len(rows) > maxImportedRows {
		return nil, nil, errors.New("匯入檢驗筆數超過上限。")
	}
	records := make([]RawRecord, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ItemName) == "" {
			return nil, nil, errors.New("檢驗項目名稱缺漏；未匯入部分結果。")
		}
		for _, text := range []string{row.CheckDate, row.OrderCode, row.OrderName, row.ItemName, row.Result, row.Unit, row.Reference, row.Institution, row.VisitDate, row.Source, row.OfficialFlag} {
			if len(text) > maxCellBytes || !utf8.ValidString(text) {
				return nil, nil, errors.New("檢驗欄位過長或不是有效UTF-8。")
			}
		}
		records = append(records, row.raw())
	}
	return records, append([]string(nil), portableWarning...), nil
}

// JSON structural validation rejects duplicate keys, deep structures and trailing
// documents before decoding medical fields. Never accept last-wins ambiguity.
func validateJSONDocument(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var read func(int) error
	read = func(depth int) error {
		if depth > 32 {
			return errors.New("JSON巢狀層級過深。")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("JSON包含重複或無效欄位。")
				}
				seen[name] = true
				if err := read(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := read(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("JSON結構不正確。")
		}
		_, err = decoder.Token()
		return err
	}
	if err := read(0); err != nil {
		return errors.New("JSON格式、層級或重複欄位不符合匯入規則。")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("JSON包含多份文件或尾隨內容。")
	}
	return nil
}
func parsePortableJSON(data []byte) ([]RawRecord, []string, error) {
	if !utf8.Valid(data) {
		return nil, nil, errors.New("JSON必須使用UTF-8。")
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if err := validateJSONDocument(data); err != nil {
		return nil, nil, err
	}
	var document struct {
		Schema  string        `json:"schema"`
		Records []portableRow `json:"records"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || document.Schema != portableSchema || document.Records == nil {
		return nil, nil, errors.New("未辨識的JSON格式。僅支援Healthbank可攜報表 healthbank.lab.v1；不是通用SDK／FHIR匯入器。")
	}
	return validatePortable(document.Records)
}
func parsePortableXML(data []byte) ([]RawRecord, []string, error) {
	if !utf8.Valid(data) {
		return nil, nil, errors.New("XML必須使用UTF-8。")
	}
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	depth := 0
	rootSeen := false
	rows := []portableRow{}
	fields := map[string]string{}
	field := ""
	var value strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, errors.New("XML不是有效、完整的可攜報表。")
		}
		switch t := token.(type) {
		case xml.Directive:
			return nil, nil, errors.New("XML不得包含DTD或實體宣告。")
		case xml.ProcInst:
			if t.Target != "xml" || rootSeen {
				return nil, nil, errors.New("XML不得包含處理指令。")
			}
		case xml.StartElement:
			depth++
			if t.Name.Space != "" {
				return nil, nil, errors.New("不支援此XML命名空間。")
			}
			switch depth {
			case 1:
				if rootSeen || t.Name.Local != "healthbank_report" || len(t.Attr) != 1 || t.Attr[0].Name.Local != "schema" || t.Attr[0].Name.Space != "" || t.Attr[0].Value != portableSchema {
					return nil, nil, errors.New("未辨識的XML格式；僅支援Healthbank可攜報表。")
				}
				rootSeen = true
			case 2:
				if t.Name.Local != "record" || len(t.Attr) != 0 {
					return nil, nil, errors.New("XML報表結構不正確。")
				}
				fields = map[string]string{}
			case 3:
				if !portableFields[t.Name.Local] || len(t.Attr) != 0 {
					return nil, nil, errors.New("XML包含不支援的檢驗欄位。")
				}
				if _, ok := fields[t.Name.Local]; ok {
					return nil, nil, errors.New("XML包含重複檢驗欄位。")
				}
				field = t.Name.Local
				value.Reset()
			default:
				return nil, nil, errors.New("XML欄位不得包含巢狀內容。")
			}
		case xml.CharData:
			if depth == 3 {
				if value.Len()+len(t) > maxCellBytes {
					return nil, nil, errors.New("XML欄位過長。")
				}
				value.Write(t)
			} else if len(bytes.TrimSpace(t)) != 0 {
				return nil, nil, errors.New("XML包含結構外文字。")
			}
		case xml.EndElement:
			if depth == 3 {
				fields[field] = value.String()
			}
			if depth == 2 {
				if len(rows) >= maxImportedRows {
					return nil, nil, errors.New("匯入檢驗筆數超過上限。")
				}
				rows = append(rows, portableRow{CheckDate: fields["check_date"], OrderCode: fields["order_code"], OrderName: fields["order_name"], ItemName: fields["item_name"], Result: fields["result"], Unit: fields["unit"], Reference: fields["reference"], Institution: fields["institution"], VisitDate: fields["visit_date"], Source: fields["source"], OfficialFlag: fields["official_flag"]})
			}
			depth--
		}
	}
	if !rootSeen || depth != 0 {
		return nil, nil, errors.New("XML報表缺漏根節點或尚未結束。")
	}
	return validatePortable(rows)
}
