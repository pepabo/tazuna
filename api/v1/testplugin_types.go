package v1

type TestPluginSpec struct {
	Type TestPluginType `json:"type"`
	// N回の連続したテスト関数の成功を、テストプラグインの通過とする
	// 指定されていない場合、一度でも成功したらOKとする
	MinConsecutiveSuccessCount int `json:"minConsecutiveSuccessCount"`
	// N回の連続したテスト関数の失敗を、テストプラグインの失敗とする
	// 指定されていない場合無視される
	MinConsecutiveFailureCount int `json:"minConsecutiveFailureCount"`
	// テストプラグイン自体の失敗とするタイムアウト秒
	// 指定されなければSuccessするまで待ち続ける
	TimeoutSeconds int `json:"timeoutSeconds"`
	// テスト関数の実行の間にいれるinterval
	// 指定されなければ即座に再実行される
	IntervalSeconds int                `json:"intervalSeconds"`
	WaitUntil       *WaitUntilArgs     `json:"waitUntil,omitempty"`
	ExistNonExist   *ExistNonExistArgs `json:"existNonExist,omitempty"`
}

type TestPluginType string

const (
	TestPluginTypeWaitUntil     TestPluginType = "WaitUntil"
	TestPluginTypeExistNonExist TestPluginType = "ExistNonExist"
)

type WaitUntilArgs struct {
	Resource  WaitUntilResource `json:"resource"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Condition string            `json:"condition"`
}

type WaitUntilResource struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

type ExistNonExistArgs struct {
	Resource    WaitUntilResource `json:"resource"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	ShouldExist bool              `json:"shouldExist"`
}
