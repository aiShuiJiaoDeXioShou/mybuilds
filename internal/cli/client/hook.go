package client

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/server"
	"net/http"
	"net/url"
	"strconv"
)

func newProjectHookCommand() *cobra.Command {
	c := &cobra.Command{Use: "hook", Short: "管理项目Webhook独立凭据及自动窗口"}
	show := &cobra.Command{Use: "show <project>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var v server.WebhookPolicyView
		if e := remoteRequest(cmd, http.MethodGet, "/api/projects/"+url.PathEscape(args[0])+"/hook", nil, &v, ""); e != nil {
			return e
		}
		return remoteOutput(cmd, v, []string{"PROVIDER", "ENABLED", "QUIET", "VERSION"}, [][]string{{v.Provider, strconv.FormatBool(v.Enabled), v.QuietPeriod, strconv.FormatInt(v.PolicyVersion, 10)}})
	}}
	show.Flags().Bool("json", false, "输出JSON")
	c.AddCommand(show)
	for _, action := range []string{"rotate", "disable", "enable"} {
		action := action
		cmd := &cobra.Command{Use: action + " <project>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			json, _ := cmd.Flags().GetBool("json")
			if action == "rotate" && !json {
				return errors.New("一次密钥响应需要json输出")
			}
			var v server.WebhookConfigured
			if e := remoteRequest(cmd, http.MethodPost, "/api/projects/"+url.PathEscape(args[0])+"/hook/"+action, struct{}{}, &v, ""); e != nil {
				return e
			}
			return remoteOutput(cmd, v, []string{"ENABLED", "VERSION"}, [][]string{{strconv.FormatBool(v.View.Enabled), strconv.FormatInt(v.View.PolicyVersion, 10)}})
		}}
		cmd.Flags().Bool("json", false, "输出JSON")
		c.AddCommand(cmd)
	}
	for _, action := range []string{"events", "windows"} {
		action := action
		cmd := &cobra.Command{Use: action + " <project>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			limit, offset, e := remotePage(cmd)
			if e != nil {
				return e
			}
			path := "/api/projects/" + url.PathEscape(args[0]) + "/hook/" + action + "?" + url.Values{"limit": {strconv.Itoa(limit)}, "offset": {strconv.Itoa(offset)}}.Encode()
			if action == "events" {
				var v struct {
					Items  []server.WebhookEventView `json:"items"`
					Limit  int                       `json:"limit"`
					Offset int                       `json:"offset"`
				}
				if e = remoteRequest(cmd, http.MethodGet, path, nil, &v, ""); e != nil {
					return e
				}
				rows := [][]string{}
				for _, r := range v.Items {
					rows = append(rows, []string{r.ID, r.Provider, r.Branch, r.Status, r.Reason, r.WindowID})
				}
				return remoteOutput(cmd, v, []string{"ID", "PROVIDER", "BRANCH", "STATUS", "REASON", "WINDOW"}, rows)
			}
			var v struct {
				Items  []server.WebhookWindowView `json:"items"`
				Limit  int                        `json:"limit"`
				Offset int                        `json:"offset"`
			}
			if e = remoteRequest(cmd, http.MethodGet, path, nil, &v, ""); e != nil {
				return e
			}
			rows := [][]string{}
			for _, r := range v.Items {
				rows = append(rows, []string{r.ID, r.Provider, r.Branch, r.State, r.Reason, r.FinalSHA})
			}
			return remoteOutput(cmd, v, []string{"ID", "PROVIDER", "BRANCH", "STATE", "REASON", "SHA"}, rows)
		}}
		remotePageFlags(cmd)
		c.AddCommand(cmd)
	}
	return c
}
