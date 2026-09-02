package agent

import "testing"

func TestBuiltinAgentPolicies(t *testing.T) {
	build, ok := Find("build")
	if !ok || !build.AllowsTool("bash") || !build.AllowsTool("write") {
		t.Fatalf("build = %#v", build)
	}
	plan, ok := Find("plan")
	if !ok || !plan.AllowsTool("read") || plan.AllowsTool("bash") || plan.AllowsTool("write") {
		t.Fatalf("plan = %#v", plan)
	}
	if _, ok := Find("missing"); ok {
		t.Fatal("missing agent was found")
	}
}
