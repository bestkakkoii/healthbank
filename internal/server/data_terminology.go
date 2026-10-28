// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import "github.com/bestkakkoii/healthbank/internal/nhi"

// These are Healthbank editorial translations of reviewed source meanings,
// not claimed official English API names. NHI terminology was cross-checked at
// https://www.nhi.gov.tw/ch/cp-7610-dc152-3046-1.html (reviewed 2026-10-28).
// Decoration is limited to public capability metadata. It never participates
// in source parsing, category selection, raw field names or clinical grading.
type dataCategoryMetadata struct {
	nhi.ProviderCategoryInfo
	LabelEN string `json:"label_en"`
}

type dataFieldTerm struct {
	Label   string `json:"label"`
	LabelEN string `json:"label_en"`
}

type dataCatalogMetadata struct {
	Categories       []dataCategoryMetadata `json:"categories"`
	FieldTerms       []dataFieldTerm        `json:"field_terms"`
	TerminologyBasis string                 `json:"terminology_basis"`
}

var dataCategoryEnglish = map[string]string{
	"adult":                          "Adult preventive care",
	"allergies":                      "Allergy records",
	"annual_overview":                "Annual care and medical radiation reference summary",
	"body_measurements":              "Source body-measurement summaries",
	"calendar_events":                "Source calendar and saved notes",
	"campaign_measurements":          "Source campaign measurement snapshots",
	"care_plans":                     "Care program participation",
	"catastrophic":                   "Catastrophic illness certificates",
	"child_preventive":               "Child preventive care records",
	"child_schedule":                 "Saved child profiles and suggested vaccination schedules",
	"chinese":                        "Traditional Chinese medicine encounters",
	"chronic":                        "Refillable prescriptions for chronic illnesses",
	"ckd_care":                       "Early chronic kidney disease care follow-up",
	"cochlear":                       "Cochlear implant records",
	"copd_care":                      "Chronic obstructive pulmonary disease care follow-up",
	"covid":                          "COVID-19 tests and vaccinations",
	"dental":                         "Dental encounters",
	"device_activity":                "Mobile-device activity records",
	"diabetes_care":                  "Diabetes care follow-up",
	"diabetes_ckd_care":              "Diabetes with early kidney disease care follow-up",
	"diet":                           "Saved dietary records",
	"diet_source_totals":             "Source dietary daily totals",
	"e_prescriptions":                "Electronic prescriptions and refills",
	"exercise":                       "Saved exercise records",
	"exercise_source_totals":         "Source exercise daily totals",
	"family_doctor":                  "Family physician program enrollment",
	"family_doctor_consent_snapshot": "Family physician program consent-page snapshot",
	"family_followup":                "Family physician laboratory follow-up",
	"family_health_followup":         "Family physician health and blood-pressure follow-up",
	"glucose":                        "Source HbA1c history",
	"health_coin_clinical":           "Health-coin source health records",
	"hepatitis_care":                 "Hepatitis B and C follow-up",
	"hepatitis_test":                 "Hepatitis C antibody testing",
	"home_care":                      "Home care enrollment",
	"hospital":                       "Inpatient records",
	"imaging":                        "Imaging and pathology reports",
	"kinmen":                         "Kinmen health examinations",
	"lifestyle_assessment":           "Saved lifestyle assessments and responses",
	"lipids":                         "Source blood-lipid history",
	"mask_collections":               "Mask purchase and collection records",
	"medicine":                       "Medication records",
	"menstrual":                      "Saved menstrual records",
	"metabolic_care":                 "Metabolic syndrome care follow-up",
	"notifications":                  "Source personalized notifications",
	"official_export_html":           "Source HTML file preparation",
	"official_export_json":           "Source JSON file preparation",
	"official_export_pdf":            "Source PDF file preparation",
	"official_export_xml":            "Source XML file preparation",
	"official_files":                 "Source-generated original files",
	"organ_wishes":                   "Organ donation and palliative-care preference annotations",
	"osteoporosis":                   "Osteoporosis examinations",
	"physiology":                     "Physiological measurement history",
	"prenatal":                       "Prenatal examination records",
	"procedure":                      "Surgery and procedures",
	"rapid_test_collections":         "Rapid-test purchase and collection records",
	"recent_chronic":                 "Recent chronic-prescription summaries",
	"recent_hospital":                "Recent inpatient summaries",
	"recent_medicine":                "Recent medication summaries",
	"recent_procedure":               "Recent surgery summaries",
	"recent_visits":                  "Source recent encounter summaries",
	"screen_breast":                  "Breast cancer screening",
	"screen_cervical":                "Cervical cancer screening",
	"screen_colorectal":              "Colorectal cancer screening",
	"screen_hpsa":                    "Helicobacter pylori stool antigen testing",
	"screen_hpv":                     "Human papillomavirus testing",
	"screen_lung":                    "Lung cancer screening",
	"screen_oral":                    "Oral cancer screening",
	"self_paid":                      "Self-paid health examinations",
	"self_paid_material":             "Institution-reported self-paid medical devices and materials",
	"self_paid_medicine":             "Institution-reported self-paid medicines",
	"source_cvd_risk":                "Saved source cardiovascular risk assessments",
	"source_diabetes_assessment":     "Source diabetes risk and education records",
	"source_disease_assessments":     "Source disease risk assessments and supporting data",
	"source_kidney_risk":             "Saved source kidney-disease assessments",
	"source_liver_risk":              "Source liver-cancer assessment snapshots",
	"stored_reminders":               "Saved source reminders and medication notifications",
	"vaccinations":                   "Vaccination records",
	"western":                        "Western medicine encounters",
}

// The terms are exact role vocabulary for display by a client. They are not
// fuzzy substitutions: an arbitrary source label, code, or value must remain
// unchanged unless that field's reviewed meaning explicitly has this role.
var dataFieldTerms = [...]dataFieldTerm{
	{Label: "檢查日期", LabelEN: "Examination date"},
	{Label: "採檢日期", LabelEN: "Specimen collection date"},
	{Label: "就醫日期", LabelEN: "Encounter date"},
	{Label: "開立日期", LabelEN: "Prescription date"},
	{Label: "領藥日期", LabelEN: "Dispensing date"},
	{Label: "住院日期", LabelEN: "Admission date"},
	{Label: "出院日期", LabelEN: "Discharge date"},
	{Label: "手術日期", LabelEN: "Surgery date"},
	{Label: "登錄日期", LabelEN: "Registration date"},
	{Label: "上傳日期", LabelEN: "Upload date"},
	{Label: "評估日期", LabelEN: "Assessment date"},
	{Label: "推播日期", LabelEN: "Notification date"},
	{Label: "來源更新日期", LabelEN: "Source update date"},
	{Label: "收案日期", LabelEN: "Care program enrollment date"},
	{Label: "結案日期", LabelEN: "Care program closure date"},
	{Label: "檢驗項目", LabelEN: "Laboratory item"},
	{Label: "檢驗結果", LabelEN: "Laboratory result"},
	{Label: "原始結果", LabelEN: "Original result"},
	{Label: "單位", LabelEN: "Unit"},
	{Label: "原院參考區間", LabelEN: "Source laboratory reference interval"},
	{Label: "原始異常旗標", LabelEN: "Original abnormal-result flag"},
	{Label: "檢體種類", LabelEN: "Specimen type"},
	{Label: "檢測方法", LabelEN: "Assay method"},
	{Label: "醫令代碼", LabelEN: "Procedure or order code"},
	{Label: "醫令名稱", LabelEN: "Procedure or order name"},
	{Label: "申報名稱", LabelEN: "Claims item name"},
	{Label: "藥品代碼", LabelEN: "Drug code"},
	{Label: "藥品名稱", LabelEN: "Drug name"},
	{Label: "成分名稱", LabelEN: "Ingredient name"},
	{Label: "劑量", LabelEN: "Dose"},
	{Label: "頻率", LabelEN: "Frequency"},
	{Label: "給藥途徑", LabelEN: "Route of administration"},
	{Label: "給藥日數", LabelEN: "Prescribed days of supply"},
	{Label: "數量", LabelEN: "Quantity"},
	{Label: "處方箋", LabelEN: "Prescription"},
	{Label: "領藥紀錄", LabelEN: "Dispensing record"},
	{Label: "實際服用", LabelEN: "Reported actual use"},
	{Label: "院所", LabelEN: "Healthcare institution"},
	{Label: "院所電話", LabelEN: "Healthcare institution telephone"},
	{Label: "部分負擔", LabelEN: "Copayment"},
	{Label: "健保點數", LabelEN: "NHI payment points"},
	{Label: "來源備註", LabelEN: "Source note"},
}

func dataCatalogWithTerminology() dataCatalogMetadata {
	items := dataCategories()
	categories := make([]dataCategoryMetadata, len(items))
	for i, item := range items {
		categories[i] = dataCategoryMetadata{ProviderCategoryInfo: item, LabelEN: dataCategoryEnglish[item.ID]}
	}
	return dataCatalogMetadata{
		Categories:       categories,
		FieldTerms:       append([]dataFieldTerm(nil), dataFieldTerms[:]...),
		TerminologyBasis: "healthbank_editorial_translation",
	}
}
