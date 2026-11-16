package isolated

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GatewayReady is created once the rules are in place; the launcher starts the
// workspace only after seeing it.
const GatewayReady = "/tmp/archetipo-gateway-ready"

// BlockedIPv4 are the destinations no process of the workspace may open a new
// connection to: the host, the engine's own networks and every local network.
var BlockedIPv4 = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

// GatewayRules is the complete egress policy, as the commands that install it.
// Loopback and replies are allowed, the ranges above are rejected and the
// internet stays open. IPv6 has no egress until the same ranges are mapped.
func GatewayRules() [][]string {
	var rules [][]string
	for _, table := range []string{"iptables", "ip6tables"} {
		rules = append(rules,
			[]string{table, "-F", "OUTPUT"},
			[]string{table, "-A", "OUTPUT", "-o", "lo", "-j", "ACCEPT"},
			[]string{table, "-A", "OUTPUT", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		)
	}
	for _, network := range BlockedIPv4 {
		rules = append(rules, []string{"iptables", "-A", "OUTPUT", "-d", network, "-j", "REJECT"})
	}
	return append(rules, []string{"ip6tables", "-A", "OUTPUT", "-j", "REJECT"})
}

// RunGateway installs the rules in the namespace the workspace will join, then
// holds that namespace until the container is stopped. A rule that fails stops
// the gateway, so the workspace is never started behind a partial policy.
func RunGateway(ctx context.Context) error {
	for _, rule := range GatewayRules() {
		if out, err := exec.CommandContext(ctx, rule[0], rule[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("gateway rule %q failed: %v: %s", strings.Join(rule, " "), err, strings.TrimSpace(string(out)))
		}
	}
	if err := os.WriteFile(GatewayReady, []byte("ready\n"), 0o644); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}
