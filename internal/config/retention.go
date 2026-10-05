package config

// Retention只用于控制端管理配置，不接受仓库流水线或节点覆盖。
type Retention struct {
	Builds int64 `yaml:"builds" mapstructure:"builds" json:"builds"`
	Days   int64 `yaml:"days" mapstructure:"days" json:"days"`
}

// nil字段逐项继承全局；空对象恢复两个字段继承。
type RetentionOverride struct {
	Builds *int64 `yaml:"builds,omitempty" json:"builds,omitempty"`
	Days   *int64 `yaml:"days,omitempty" json:"days,omitempty"`
}

func ValidateRetention(policy Retention) error {
	if policy.Builds <= 0 {
		return invalid("retention.builds", "需要正整数")
	}
	if policy.Days <= 0 || policy.Days > 106751 {
		return invalid("retention.days", "需要1到106751的整数")
	}
	return nil
}

func ValidateRetentionOverride(policy *RetentionOverride) error {
	if policy == nil {
		return nil
	}
	if policy.Builds != nil && *policy.Builds <= 0 {
		return invalid("retention.builds", "需要正整数")
	}
	if policy.Days != nil && (*policy.Days <= 0 || *policy.Days > 106751) {
		return invalid("retention.days", "需要1到106751的整数")
	}
	return nil
}
