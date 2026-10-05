package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"mybuilds/internal/config"
	"mybuilds/internal/distribute"
	"mybuilds/internal/protocol"
)

type publishDoctorOptions struct{ Target, AgentConfig, AppID, CredentialsEnv, Certificate string }

func publishDoctor(cmd *cobra.Command, o publishDoctorOptions, asJSON bool) error {
	if o.Target != "google-play" && o.Target != "app-store" {
		return errors.New("发布诊断目标无效")
	}
	if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(o.CredentialsEnv) || strings.HasPrefix(o.CredentialsEnv, "MYBUILDS_") {
		return errors.New("需要明确的发布材料环境变量")
	}
	credential, ok := os.LookupEnv(o.CredentialsEnv)
	if !ok || credential == "" {
		return errors.New("发布材料环境变量缺失")
	}
	tools, dir, err := config.LoadPublishDoctor(o.AgentConfig)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	task := protocol.PublishQueryTask{ID: uuid.NewString(), Nonce: uuid.NewString(), BindingID: uuid.NewString(), Kind: "doctor", AppIdentifier: o.AppID, ExpiresAt: time.Now().UTC().Add(30 * time.Second)}
	var result protocol.PublishQueryResult
	if o.Target == "google-play" {
		task.Store = "google_play"
		result, err = distribute.QueryGooglePlay(ctx, distribute.GooglePlayOptions{BundleDir: tools.BundleDir, Bundletool: tools.Bundletool, DataDir: dir, CredentialFile: credential, AppIdentifier: o.AppID, UploadCertificateSHA256: o.Certificate}, task)
	} else {
		task.Store = "app_store"
		result, err = distribute.QueryApple(ctx, distribute.AppleOptions{BundleDir: tools.BundleDir, DataDir: dir, CredentialFile: credential, AppIdentifier: o.AppID}, task)
	}
	if err != nil {
		return errors.New("发布环境检查未通过")
	}
	if asJSON {
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result.DoctorChecks); err != nil {
			return errors.New("写入检查结果失败")
		}
	} else {
		for _, c := range result.DoctorChecks {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s %s %s\n", c.Name, c.Status, c.Version, c.Reason); err != nil {
				return errors.New("写入检查结果失败")
			}
		}
	}
	if result.Reason != "" || len(result.DoctorChecks) != 4 {
		return errors.New("发布环境检查未通过")
	}
	for _, c := range result.DoctorChecks {
		if c.Status != "passed" {
			return errors.New("发布环境检查未通过")
		}
	}
	return nil
}
