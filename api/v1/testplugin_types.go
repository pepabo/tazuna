package v1

type TestPluginSpec struct {
	Type TestPluginType `yaml:"type"`
	// N回の連続したテスト関数の成功を、テストプラグインの通過とする
	// 指定されていない場合、一度でも成功したらOKとする
	MinConsecutiveSuccessCount int `yaml:"minConsecutiveSuccessCount"`
	// N回の連続したテスト関数の失敗を、テストプラグインの失敗とする
	// 指定されていない場合無視される
	MinConsecutiveFailureCount int `yaml:"minConsecutiveFailureCount"`
	// テストプラグイン自体の失敗とするタイムアウト秒
	// 指定されなければSuccessするまで待ち続ける
	TimeoutSeconds int `yaml:"timeoutSeconds"`
	// テスト関数の実行の間にいれるinterval
	// 指定されなければ即座に再実行される
	IntervalSeconds int                `yaml:"intervalSeconds"`
	WaitUntil       *WaitUntilArgs     `yaml:"waitUntil,omitempty"`
	ExistNonExist   *ExistNonExistArgs `yaml:"existNonExist,omitempty"`
}

type TestPluginType string

const (
	TestPluginTypeWaitUntil     TestPluginType = "WaitUntil"
	TestPluginTypeExistNonExist TestPluginType = "ExistNonExist"
)

type WaitUntilArgs struct {
	Resource  WaitUntilResource `yaml:"resource"`
	Namespace string            `yaml:"namespace"`
	Name      string            `yaml:"name"`
	Condition string            `yaml:"condition"`
}

type WaitUntilResource struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
}

type ExistNonExistArgs struct {
	Resource    WaitUntilResource `yaml:"resource"`
	Namespace   string            `yaml:"namespace"`
	Name        string            `yaml:"name"`
	ShouldExist bool              `yaml:"shouldExist"`
}
