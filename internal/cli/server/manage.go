package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	"mybuilds/internal/store"
	"text/tabwriter"
)

var localAdmin = store.Actor{ID: "local-admin", Role: "admin"}

// withStore让所有本机管理操作取得与serve相同的运行权。
func withStore(cmd *cobra.Command, fn func(*store.Store) error) (err error) {
	filename, _ := cmd.Flags().GetString("config")
	cfg, err := config.LoadServer(config.ServerLoadOptions{Filename: filename, Explicit: cmd.Flags().Changed("config")})
	if err != nil {
		return err
	}
	db, err := store.Open(cmd.Context(), store.Options{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if err = db.Migrate(cmd.Context()); err != nil {
		return err
	}
	return fn(db)
}
func newMigrateCommand() *cobra.Command {
	return &cobra.Command{Use: "migrate", Short: "取得独占并迁移数据库", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return withStore(cmd, func(_ *store.Store) error { return nil })
	}}
}
func managementOutput(cmd *cobra.Command, value any, columns []string, rows [][]string) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(value); err != nil {
			return errors.New("写入管理结果失败")
		}
		return nil
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	for _, row := range append([][]string{columns}, rows...) {
		for i, item := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, item)
		}
		fmt.Fprintln(w)
	}
	if err := w.Flush(); err != nil {
		return errors.New("写入管理结果失败")
	}
	return nil
}
func pageFlags(cmd *cobra.Command) {
	cmd.Flags().Int("limit", 20, "每页数量，最大200")
	cmd.Flags().Int("offset", 0, "分页起点")
	cmd.Flags().Bool("json", false, "输出JSON")
}
func managementPage(cmd *cobra.Command) (store.Page, error) {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	if limit < 1 || limit > 200 || offset < 0 || offset > 1000000 {
		return store.Page{}, errors.New("分页参数不合法")
	}
	return store.Page{Limit: limit, Offset: offset}, nil
}
func newGroupCommand() *cobra.Command {
	group := &cobra.Command{Use: "group", Short: "管理项目组"}
	create := &cobra.Command{Use: "create <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(cmd, func(db *store.Store) error {
			result, err := db.CreateGroup(cmd.Context(), localAdmin, args[0])
			if err != nil {
				return err
			}
			return managementOutput(cmd, result, []string{"ID", "NAME"}, [][]string{{result.ID, result.Name}})
		})
	}}
	create.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		page, err := managementPage(cmd)
		if err != nil {
			return err
		}
		return withStore(cmd, func(db *store.Store) error {
			items, err := db.ListGroups(cmd.Context(), page)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(items))
			for _, v := range items {
				rows = append(rows, []string{v.ID, v.Name})
			}
			return managementOutput(cmd, struct {
				Items  []store.Group `json:"items"`
				Limit  int           `json:"limit"`
				Offset int           `json:"offset"`
			}{items, page.Limit, page.Offset}, []string{"ID", "NAME"}, rows)
		})
	}}
	pageFlags(list)
	rename := &cobra.Command{Use: "rename <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			return errors.New("需要新组名")
		}
		return withStore(cmd, func(db *store.Store) error {
			result, err := db.RenameGroup(cmd.Context(), localAdmin, args[0], name)
			if err != nil {
				return err
			}
			return managementOutput(cmd, result, []string{"ID", "NAME"}, [][]string{{result.ID, result.Name}})
		})
	}}
	rename.Flags().String("name", "", "新组名")
	rename.Flags().Bool("json", false, "输出JSON")
	remove := &cobra.Command{Use: "rm <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(cmd, func(db *store.Store) error { return db.DeleteGroup(cmd.Context(), localAdmin, args[0]) })
	}}
	group.AddCommand(create, list, rename, remove)
	return group
}
func newTokenCommand() *cobra.Command {
	token := &cobra.Command{Use: "token", Short: "管理身份令牌"}
	create := &cobra.Command{Use: "create", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		role, _ := cmd.Flags().GetString("role")
		return withStore(cmd, func(db *store.Store) error {
			result, err := db.CreateToken(cmd.Context(), localAdmin, role)
			if err != nil {
				return err
			}
			return managementOutput(cmd, result, []string{"ID", "ROLE", "TOKEN"}, [][]string{{result.ID, result.Role, result.Token}})
		})
	}}
	create.Flags().String("role", "", "admin/trigger/approver")
	create.Flags().Bool("json", false, "输出JSON")
	list := &cobra.Command{Use: "ls", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		page, err := managementPage(cmd)
		if err != nil {
			return err
		}
		return withStore(cmd, func(db *store.Store) error {
			items, err := db.ListTokens(cmd.Context(), localAdmin, page)
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(items))
			for _, v := range items {
				rows = append(rows, []string{v.ID, v.Role})
			}
			return managementOutput(cmd, struct {
				Items  []store.TokenView `json:"items"`
				Limit  int               `json:"limit"`
				Offset int               `json:"offset"`
			}{items, page.Limit, page.Offset}, []string{"ID", "ROLE"}, rows)
		})
	}}
	pageFlags(list)
	revoke := &cobra.Command{Use: "revoke <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withStore(cmd, func(db *store.Store) error { return db.RevokeToken(cmd.Context(), localAdmin, args[0]) })
	}}
	token.AddCommand(create, list, revoke)
	return token
}
