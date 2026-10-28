// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"net/http"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

// The catalog is public capability metadata, never a list of this person's
// available records or a promise that an upstream account has every category.
func dataCategories() []nhi.ProviderCategoryInfo {
	items := []nhi.ProviderCategoryInfo{
		{ID: "medicine", Label: "用藥紀錄", Group: "用藥與就醫", RangeMode: "date_search", HasDetails: true},
		{ID: "chronic", Label: "慢性病連續處方箋", Group: "用藥與就醫", RangeMode: "source", HasDetails: true},
		{ID: "western", Label: "西醫就醫", Group: "用藥與就醫", RangeMode: "date_search", HasDetails: true},
		{ID: "chinese", Label: "中醫就醫", Group: "用藥與就醫", RangeMode: "date_search", HasDetails: true},
		{ID: "dental", Label: "牙醫就醫", Group: "用藥與就醫", RangeMode: "date_search", HasDetails: true},
		{ID: "procedure", Label: "手術與處置", Group: "用藥與就醫", RangeMode: "date_search", HasDetails: true},
		{ID: "hospital", Label: "住院紀錄", Group: "用藥與就醫", RangeMode: "date_search", HasDetails: true},
		{ID: "adult", Label: "成人預防保健", Group: "檢查與報告", RangeMode: "source"},
		{ID: "self_paid", Label: "自費健檢", Group: "檢查與報告", RangeMode: "source", HasDetails: true},
		{ID: "imaging", Label: "影像與病理報告", Group: "檢查與報告", RangeMode: "source", HasDetails: true},
		{ID: "recent_medicine", Label: "近期用藥摘要", Group: "官方近期摘要", RangeMode: "source", HasDetails: true},
		{ID: "recent_chronic", Label: "近期慢箋摘要", Group: "官方近期摘要", RangeMode: "source", HasDetails: true},
		{ID: "recent_procedure", Label: "近期手術摘要", Group: "官方近期摘要", RangeMode: "source", HasDetails: true},
		{ID: "recent_hospital", Label: "近期住院摘要", Group: "官方近期摘要", RangeMode: "source", HasDetails: true},
	}
	items = append(items, nhi.ProviderCategories()...)
	items = append(items,
		nhi.ProviderCategoryInfo{ID: "official_files", Label: "官方已產製原始檔", Group: "原始文件", RangeMode: "source", HasDetails: true},
		nhi.ProviderCategoryInfo{ID: "official_export_html", Label: "官方 HTML 原始檔產製", Group: "原始文件", RangeMode: "source"},
		nhi.ProviderCategoryInfo{ID: "official_export_xml", Label: "官方 XML 原始檔產製", Group: "原始文件", RangeMode: "source"},
		nhi.ProviderCategoryInfo{ID: "official_export_json", Label: "官方 JSON 原始檔產製", Group: "原始文件", RangeMode: "source"},
		nhi.ProviderCategoryInfo{ID: "official_export_pdf", Label: "官方 PDF 原始檔產製", Group: "原始文件", RangeMode: "source"},
	)
	return items
}

func (a *App) dataCatalog(w http.ResponseWriter, _ *http.Request) {
	a.json(w, http.StatusOK, dataCatalogWithTerminology())
}
