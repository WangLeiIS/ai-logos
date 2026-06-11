package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

var (
	searchTag string
)

var searchCmd = &cobra.Command{
	Use:   "search <keyword> [--tag <tag>]",
	Short: "Search packages on irollhub",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		keyword := args[0]

		// 读取配置
		config, err := readConfig()
		if err != nil {
			outputError(fmt.Sprintf("Failed to read config: %v", err))
		}

		client := NewHubClient(config.HubURL, "")

		// 构造查询 URL
		query := fmt.Sprintf("?q=%s", url.QueryEscape(keyword))
		if searchTag != "" {
			query += fmt.Sprintf("&tag=%s", url.QueryEscape(searchTag))
		}

		// 发送请求
		var resp *http.Response
		err = retry(func() error {
			var err error
			resp, err = client.Get("/api/v1/search" + query)
			return err
		}, MaxRetries)

		if err != nil {
			outputError(fmt.Sprintf("Network error: %v", err))
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			apiErr := parseAPIError(resp)
			outputError(fmt.Sprintf("Search failed: %v", apiErr))
		}

		// 解析响应
		var result struct {
			Results []struct {
				Org       string `json:"org"`
				Package   string `json:"package"`
				Version   string `json:"version"`
				Downloads int    `json:"downloads"`
				OrgName   string `json:"org_name"`
				PkgName   string `json:"pkg_name"`
				PkgDesc   string `json:"pkg_desc"`
			} `json:"results"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			outputError(fmt.Sprintf("Failed to parse response: %v", err))
		}

		// 格式化输出
		type SearchResult struct {
			Org       string `json:"org"`
			Package   string `json:"package"`
			Version   string `json:"version"`
			Downloads int    `json:"downloads"`
			OrgName   string `json:"org_name"`
			PkgName   string `json:"pkg_name"`
			PkgDesc   string `json:"pkg_desc"`
		}

		outputs := make([]SearchResult, len(result.Results))
		for i, r := range result.Results {
			outputs[i] = SearchResult{
				Org:       r.Org,
				Package:   r.Package,
				Version:   r.Version,
				Downloads: r.Downloads,
				OrgName:   r.OrgName,
				PkgName:   r.PkgName,
				PkgDesc:   r.PkgDesc,
			}
		}

		outputJSON(map[string]interface{}{
			"results": outputs,
		})
	},
}

func init() {
	searchCmd.Flags().StringVar(&searchTag, "tag", "", "Filter by tag")
	rollCmd.AddCommand(searchCmd)
}
