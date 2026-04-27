package v1

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTestPluginSpec_WaitUntil_RoundTrip(t *testing.T) {
	t.Parallel()
	src := TestPluginSpec{
		Type:                       TestPluginTypeWaitUntil,
		MinConsecutiveSuccessCount: 3,
		MinConsecutiveFailureCount: 5,
		TimeoutSeconds:             60,
		IntervalSeconds:            2,
		WaitUntil: &WaitUntilArgs{
			Resource:  WaitUntilResource{APIVersion: "apps/v1", Kind: "Deployment"},
			Namespace: "default",
			Name:      "nginx",
			Condition: "Available",
		},
	}

	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got TestPluginSpec
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nyaml:\n%s", err, string(b))
	}

	if got.Type != TestPluginTypeWaitUntil {
		t.Errorf("Type = %q", got.Type)
	}
	if got.MinConsecutiveSuccessCount != 3 || got.MinConsecutiveFailureCount != 5 {
		t.Errorf("Counts = success:%d failure:%d", got.MinConsecutiveSuccessCount, got.MinConsecutiveFailureCount)
	}
	if got.TimeoutSeconds != 60 || got.IntervalSeconds != 2 {
		t.Errorf("Timeout=%d Interval=%d", got.TimeoutSeconds, got.IntervalSeconds)
	}
	if got.WaitUntil == nil {
		t.Fatal("WaitUntil nil")
	}
	if got.WaitUntil.Resource.APIVersion != "apps/v1" || got.WaitUntil.Resource.Kind != "Deployment" {
		t.Errorf("Resource = %+v", got.WaitUntil.Resource)
	}
	if got.WaitUntil.Namespace != "default" || got.WaitUntil.Name != "nginx" || got.WaitUntil.Condition != "Available" {
		t.Errorf("WaitUntil = %+v", got.WaitUntil)
	}
	if got.ExistNonExist != nil {
		t.Errorf("ExistNonExist = %+v, want nil", got.ExistNonExist)
	}
}

func TestTestPluginSpec_ExistNonExist_RoundTrip(t *testing.T) {
	t.Parallel()
	src := TestPluginSpec{
		Type:            TestPluginTypeExistNonExist,
		IntervalSeconds: 1,
		ExistNonExist: &ExistNonExistArgs{
			Resource:    WaitUntilResource{APIVersion: "v1", Kind: "ConfigMap"},
			Namespace:   "kube-system",
			Name:        "cluster-info",
			ShouldExist: true,
		},
	}

	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got TestPluginSpec
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if got.Type != TestPluginTypeExistNonExist {
		t.Errorf("Type = %q", got.Type)
	}
	if got.ExistNonExist == nil {
		t.Fatal("ExistNonExist nil")
	}
	if !got.ExistNonExist.ShouldExist {
		t.Error("ShouldExist = false, want true")
	}
	if got.ExistNonExist.Resource.Kind != "ConfigMap" {
		t.Errorf("Resource.Kind = %q", got.ExistNonExist.Resource.Kind)
	}
	if got.WaitUntil != nil {
		t.Errorf("WaitUntil = %+v, want nil", got.WaitUntil)
	}
}
