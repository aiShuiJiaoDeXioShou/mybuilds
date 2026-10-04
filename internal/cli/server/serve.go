package server

import (
	"errors"
	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	control "mybuilds/internal/server"
	"mybuilds/internal/store"
	"os"
	"os/signal"
	"syscall"
)

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "serve", Short: "启动仅排队的控制端", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) (err error) {
		filename, _ := cmd.Flags().GetString("config")
		options := config.ServerLoadOptions{Filename: filename, Explicit: cmd.Flags().Changed("config")}
		if cmd.Flags().Changed("listen") {
			value, _ := cmd.Flags().GetString("listen")
			options.CLI.Listen = &value
		}
		if cmd.Flags().Changed("data-dir") {
			value, _ := cmd.Flags().GetString("data-dir")
			options.CLI.DataDir = &value
		}
		if cmd.Flags().Changed("concurrency") {
			value, _ := cmd.Flags().GetInt("concurrency")
			options.CLI.Concurrency = &value
		}
		cfg, err := config.LoadServer(options)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
			return errors.New("无法创建私有数据目录")
		}
		info, err := os.Stat(cfg.DataDir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			return errors.New("数据目录需要0700权限")
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		db, err := store.Open(ctx, store.Options{Driver: cfg.Database.Driver, DSN: cfg.Database.DSN})
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := db.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}()
		if err = db.Migrate(ctx); err != nil {
			return err
		}
		if err = db.Bootstrap(ctx, os.Getenv("MYBUILDS_BOOTSTRAP_ADMIN_TOKEN")); err != nil {
			return err
		}
		return control.New(db, cfg).ListenAndServe(ctx)
	}}
	cmd.Flags().String("listen", "", "监听地址")
	cmd.Flags().String("data-dir", "", "数据目录")
	cmd.Flags().Int("concurrency", 1, "控制端并发上限")
	return cmd
}
