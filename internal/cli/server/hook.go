package server

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	control "mybuilds/internal/server"
	"mybuilds/internal/store"
	"strconv"
)

func newLocalHookCommand() *cobra.Command {
	c := &cobra.Command{Use: "hook", Short: "离线管理项目Webhook，在线须用远程命令"}
	for _, action := range []string{"show", "enable", "disable", "rotate"} {
		action := action
		cmd := &cobra.Command{Use: action + " <project>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			json, _ := cmd.Flags().GetBool("json")
			if action == "rotate" && !json {
				return errors.New("一次密钥响应需要json输出")
			}
			return withStore(cmd, func(st *store.Store) error {
				if action == "show" {
					p, e := st.ReadWebhookPolicy(cmd.Context(), args[0])
					if e != nil {
						return e
					}
					v := control.WebhookPolicyView{Enabled: p.Enabled, Provider: p.Provider, RepositoryKey: p.RepositoryKey, Builds: p.BuildNames, QuietPeriod: p.QuietPeriod.String(), AllowUpload: p.AllowUpload, PolicyVersion: p.PolicyVersion}
					return managementOutput(cmd, v, []string{"PROVIDER", "ENABLED", "VERSION"}, [][]string{{p.Provider, strconv.FormatBool(p.Enabled), strconv.FormatInt(p.PolicyVersion, 10)}})
				}
				p, e := st.GetProject(cmd.Context(), args[0])
				if e != nil {
					return e
				}
				settings := p.Settings
				if settings.Hook == nil {
					return store.ErrNotFound
				}
				hook := *settings.Hook
				settings.Hook = &hook
				if action == "disable" {
					hook.Enabled = false
				}
				if action == "enable" {
					hook.Enabled = true
				}
				filename, _ := cmd.Flags().GetString("config")
				cfg, e := config.LoadServer(config.ServerLoadOptions{Filename: filename, Explicit: cmd.Flags().Changed("config")})
				if e != nil {
					return e
				}
				v, e := control.New(st, cfg).ConfigureWebhook(cmd.Context(), localAdmin, args[0], settings, action == "rotate")
				if e != nil {
					return e
				}
				return managementOutput(cmd, v, []string{"ENABLED", "VERSION"}, [][]string{{strconv.FormatBool(v.View.Enabled), strconv.FormatInt(v.View.PolicyVersion, 10)}})
			})
		}}
		cmd.Flags().Bool("json", false, "输出JSON")
		c.AddCommand(cmd)
	}
	for _, action := range []string{"events", "windows"} {
		action := action
		cmd := &cobra.Command{Use: action + " <project>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			page, err := managementPage(cmd)
			if err != nil {
				return err
			}
			return withStore(cmd, func(st *store.Store) error {
				rows := [][]string{}
				if action == "events" {
					values, err := st.ListWebhookEvents(cmd.Context(), localAdmin, args[0], page)
					if err != nil {
						return err
					}
					views := make([]control.WebhookEventView, 0, len(values))
					for _, value := range values {
						v := control.WebhookEventSummary(value)
						views = append(views, v)
						rows = append(rows, []string{v.ID, v.Provider, v.Branch, v.Status, v.Reason, v.WindowID})
					}
					out := struct {
						Items  []control.WebhookEventView `json:"items"`
						Limit  int                        `json:"limit"`
						Offset int                        `json:"offset"`
					}{views, page.Limit, page.Offset}
					return managementOutput(cmd, out, []string{"ID", "PROVIDER", "BRANCH", "STATUS", "REASON", "WINDOW"}, rows)
				}
				values, err := st.ListWebhookWindows(cmd.Context(), localAdmin, args[0], page)
				if err != nil {
					return err
				}
				views := make([]control.WebhookWindowView, 0, len(values))
				for _, value := range values {
					v := control.WebhookWindowSummary(value)
					views = append(views, v)
					rows = append(rows, []string{v.ID, v.Provider, v.Branch, v.State, v.Reason, v.FinalSHA})
				}
				out := struct {
					Items  []control.WebhookWindowView `json:"items"`
					Limit  int                         `json:"limit"`
					Offset int                         `json:"offset"`
				}{views, page.Limit, page.Offset}
				return managementOutput(cmd, out, []string{"ID", "PROVIDER", "BRANCH", "STATE", "REASON", "SHA"}, rows)
			})
		}}
		pageFlags(cmd)
		c.AddCommand(cmd)
	}
	return c
}
