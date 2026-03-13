package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Go 1.26: 验证错误集合 - 收集所有验证错误而不是只返回第一个
// 这个SB结构让调用者能看到所有问题，不是只看到第一个
type ValidationErrors []string

// Error 实现 error 接口
func (ve ValidationErrors) Error() string {
	if len(ve) == 1 {
		return ve[0]
	}
	return fmt.Sprintf("验证失败（%d个错误）:\n- %s", len(ve), strings.Join(ve, "\n- "))
}

// Is 指示这是验证错误类型
func (ve ValidationErrors) Is(target error) bool {
	_, ok := target.(ValidationErrors)
	return ok
}

// Go 1.26: 使用 errors.As 优化类型检查的辅助类型
var (
	// ErrValidation 验证错误哨兵值，方便 errors.Is 检查
	ErrValidation = errors.New("验证失败")
)

// SiteItem 一条站点数据
type SiteItem struct {
	SiteName    string `json:"siteName"`
	SiteID      string `json:"siteid"`
	Duplication string `json:"duplication"`
	Size        string `json:"size"`
	ID          int    `json:"id"`
}

// Validate 验证单条数据正确性
// Go 1.26: 收集所有验证错误，而不是遇到第一个就返回
func (s *SiteItem) Validate() error {
	var errs ValidationErrors

	if strings.TrimSpace(s.SiteName) == "" {
		errs = append(errs, errSiteNameEmpty)
	}

	if s.SiteID == "" {
		errs = append(errs, errSiteIDEmpty)
	} else if _, err := strconv.ParseInt(s.SiteID, 10, 64); err != nil {
		errs = append(errs, fmt.Sprintf("%s: %s", errSiteIDInvalid, s.SiteID))
	}

	if s.Duplication != "" {
		if _, err := strconv.ParseFloat(s.Duplication, 64); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", errDuplicationInvalid, s.Duplication))
		}
	}

	if s.Size != "" && !sizePattern.MatchString(s.Size) {
		errs = append(errs, fmt.Sprintf("%s: %s", errSizeInvalid, s.Size))
	}

	if s.ID <= 0 {
		errs = append(errs, fmt.Sprintf("%s: %d", errIDInvalid, s.ID))
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ProcessedData 完整的Top1000数据
type ProcessedData struct {
	Time  string     `json:"time"`
	Items []SiteItem `json:"items"`
}

// Validate 验证完整数据
// Go 1.26: 收集所有验证错误，包括所有子项的错误
func (p *ProcessedData) Validate() error {
	var errs ValidationErrors

	if strings.TrimSpace(p.Time) == "" {
		errs = append(errs, errTimeEmpty)
	}

	if len(p.Items) == 0 {
		errs = append(errs, errItemsEmpty)
	}

	// 验证所有条目，收集所有错误
	for i, item := range p.Items {
		if err := item.Validate(); err != nil {
			// 如果是 ValidationErrors，展开它们
			if ve, ok := err.(ValidationErrors); ok {
				for _, e := range ve {
					errs = append(errs, fmt.Sprintf("%s: %s", fmt.Sprintf(errItemValidateFailed, i+1), e))
				}
			} else {
				errs = append(errs, fmt.Sprintf("%s: %s", fmt.Sprintf(errItemValidateFailed, i+1), err))
			}
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}
