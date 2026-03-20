package model

import (
	"strings"
	"testing"
)

func TestSiteItem_Validate(t *testing.T) {
	tests := []struct {
		name    string
		item    SiteItem
		wantErr bool
		errMsg  string
	}{
		{
			name: "有效数据",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: false,
		},
		{
			name: "站点名称为空",
			item: SiteItem{
				SiteName:    "",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: true,
			errMsg:  "站点名称不能为空",
		},
		{
			name: "站点名称仅空格",
			item: SiteItem{
				SiteName:    "   ",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: true,
			errMsg:  "站点名称不能为空",
		},
		{
			name: "站点ID为空",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "",
				Duplication: "85.5",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: true,
			errMsg:  "站点ID不能为空",
		},
		{
			name: "站点ID非数字",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "abc",
				Duplication: "85.5",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: true,
			errMsg:  "站点ID必须是数字",
		},
		{
			name: "重复度非数字",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "abc",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: true,
			errMsg:  "重复度必须为数字",
		},
		{
			name: "文件大小格式错误",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "invalid",
				ID:          1,
			},
			wantErr: true,
			errMsg:  "文件大小格式错误",
		},
		{
			name: "ID为0",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "1.2TB",
				ID:          0,
			},
			wantErr: true,
			errMsg:  "ID必须大于0",
		},
		{
			name: "ID为负数",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "1.2TB",
				ID:          -1,
			},
			wantErr: true,
			errMsg:  "ID必须大于0",
		},
		{
			name: "空重复度(允许)",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: false,
		},
		{
			name: "百分比重复度(允许)",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5%",
				Size:        "1.2TB",
				ID:          1,
			},
			wantErr: false,
		},
		{
			name: "空大小(允许)",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "",
				ID:          1,
			},
			wantErr: false,
		},
		{
			name: "各种大小格式",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "1.5GB",
				ID:          1,
			},
			wantErr: false,
		},
		{
			name: "带小数的大小",
			item: SiteItem{
				SiteName:    "测试站点",
				SiteID:      "123",
				Duplication: "85.5",
				Size:        "1.23 TB",
				ID:          1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.item.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SiteItem.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if err.Error() != tt.errMsg && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("SiteItem.Validate() error = %v, 期望包含 %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestProcessedData_Validate(t *testing.T) {
	tests := []struct {
		name    string
		data    ProcessedData
		wantErr bool
		errMsg  string
	}{
		{
			name: "有效数据",
			data: ProcessedData{
				Time: "2026-01-19 07:50:56",
				Items: []SiteItem{
					{
						SiteName:    "测试站点",
						SiteID:      "123",
						Duplication: "85.5",
						Size:        "1.2TB",
						ID:          1,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "时间为空",
			data: ProcessedData{
				Time: "",
				Items: []SiteItem{
					{
						SiteName: "测试站点",
						SiteID:   "123",
						ID:       1,
					},
				},
			},
			wantErr: true,
			errMsg:  "时间不能为空",
		},
		{
			name: "时间仅空格",
			data: ProcessedData{
				Time: "   ",
				Items: []SiteItem{
					{
						SiteName: "测试站点",
						SiteID:   "123",
						ID:       1,
					},
				},
			},
			wantErr: true,
			errMsg:  "时间不能为空",
		},
		{
			name: "Items为空",
			data: ProcessedData{
				Time:  "2026-01-19 07:50:56",
				Items: []SiteItem{},
			},
			wantErr: true,
			errMsg:  "数据条目不能为空",
		},
		{
			name: "Items为nil",
			data: ProcessedData{
				Time:  "2026-01-19 07:50:56",
				Items: nil,
			},
			wantErr: true,
			errMsg:  "数据条目不能为空",
		},
		{
			name: "包含无效的Item",
			data: ProcessedData{
				Time: "2026-01-19 07:50:56",
				Items: []SiteItem{
					{
						SiteName: "测试站点",
						SiteID:   "123",
						ID:       0,
					},
				},
			},
			wantErr: true,
			errMsg:  "第1条数据验证失败",
		},
		{
			name: "多个Items，第二个无效",
			data: ProcessedData{
				Time: "2026-01-19 07:50:56",
				Items: []SiteItem{
					{
						SiteName: "测试站点1",
						SiteID:   "123",
						ID:       1,
					},
					{
						SiteName: "",
						SiteID:   "456",
						ID:       2,
					},
				},
			},
			wantErr: true,
			errMsg:  "第2条数据验证失败",
		},
		{
			name: "多个有效Items",
			data: ProcessedData{
				Time: "2026-01-19 07:50:56",
				Items: []SiteItem{
					{
						SiteName: "测试站点1",
						SiteID:   "123",
						ID:       1,
					},
					{
						SiteName: "测试站点2",
						SiteID:   "456",
						ID:       2,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.data.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessedData.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" {
				if err.Error() != tt.errMsg && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ProcessedData.Validate() error = %v, 期望包含 %v", err, tt.errMsg)
				}
			}
		})
	}
}
