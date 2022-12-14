package forward

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/coredns/caddy"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		input           string
		shouldErr       bool
		expectedFrom    string
		expectedIgnored []string
		expectedFails   uint32
		expectedOpts    options
		expectedErr     string
	}{
		// positive
		{"forward . 127.0.0.1", false, ".", nil, 2, options{hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1 {\nexcept miek.nl\n}\n", false, ".", nil, 2, options{hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1 {\nmax_fails 3\n}\n", false, ".", nil, 3, options{hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1 {\nforce_tcp\n}\n", false, ".", nil, 2, options{forceTCP: true, hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1 {\nprefer_udp\n}\n", false, ".", nil, 2, options{preferUDP: true, hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1 {\nforce_tcp\nprefer_udp\n}\n", false, ".", nil, 2, options{preferUDP: true, forceTCP: true, hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1:53", false, ".", nil, 2, options{hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1:8080", false, ".", nil, 2, options{hcRecursionDesired: true}, ""},
		{"forward . [::1]:53", false, ".", nil, 2, options{hcRecursionDesired: true}, ""},
		{"forward . [2003::1]:53", false, ".", nil, 2, options{hcRecursionDesired: true}, ""},
		{"forward . 127.0.0.1 \n", false, ".", nil, 2, options{hcRecursionDesired: true}, ""},
		{"forward 10.9.3.0/18 127.0.0.1", false, "0.9.10.in-addr.arpa.", nil, 2, options{hcRecursionDesired: true}, ""},
		// negative
		{"forward . a27.0.0.1", true, "", nil, 0, options{hcRecursionDesired: true}, "not an IP"},
		{"forward . 127.0.0.1 {\nblaatl\n}\n", true, "", nil, 0, options{hcRecursionDesired: true}, "unknown property"},
		{`forward . ::1
		forward com ::2`, true, "", nil, 0, options{hcRecursionDesired: true}, "plugin"},
		{"forward . https://127.0.0.1 \n", true, ".", nil, 2, options{hcRecursionDesired: true}, "'https' is not supported as a destination protocol in forward: https://127.0.0.1"},
		{"forward xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx 127.0.0.1 \n", true, ".", nil, 2, options{hcRecursionDesired: true}, "unable to normalize 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Fatalf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && f.from != test.expectedFrom {
			t.Errorf("Test %d: expected: %s, got: %s", i, test.expectedFrom, f.from)
		}
		if !test.shouldErr && test.expectedIgnored != nil {
			if !reflect.DeepEqual(f.ignored, test.expectedIgnored) {
				t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedIgnored, f.ignored)
			}
		}
		if !test.shouldErr && f.maxfails != test.expectedFails {
			t.Errorf("Test %d: expected: %d, got: %d", i, test.expectedFails, f.maxfails)
		}
		if !test.shouldErr && f.opts != test.expectedOpts {
			t.Errorf("Test %d: expected: %v, got: %v", i, test.expectedOpts, f.opts)
		}
	}
}

func TestSetupTLS(t *testing.T) {
	tests := []struct {
		input              string
		shouldErr          bool
		expectedServerName string
		expectedErr        string
	}{
		// positive
		{`forward . tls://127.0.0.1 {
				tls_servername dns
			}`, false, "dns", ""},
		{`forward . 127.0.0.1 {
				tls_servername dns
			}`, false, "", ""},
		{`forward . 127.0.0.1 {
				tls
			}`, false, "", ""},
		{`forward . tls://127.0.0.1`, false, "", ""},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && test.expectedServerName != "" && test.expectedServerName != f.tlsConfig.ServerName {
			t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedServerName, f.tlsConfig.ServerName)
		}

		if !test.shouldErr && test.expectedServerName != "" && test.expectedServerName != f.proxies[0].health.(*dnsHc).c.TLSConfig.ServerName {
			t.Errorf("Test %d: expected: %q, actual: %q", i, test.expectedServerName, f.proxies[0].health.(*dnsHc).c.TLSConfig.ServerName)
		}
	}
}

func TestSetupResolvconf(t *testing.T) {
	const resolv = "resolv.conf"
	if err := os.WriteFile(resolv,
		[]byte(`nameserver 10.10.255.252
nameserver 10.10.255.253`), 0666); err != nil {
		t.Fatalf("Failed to write resolv.conf file: %s", err)
	}
	defer os.Remove(resolv)

	tests := []struct {
		input         string
		shouldErr     bool
		expectedErr   string
		expectedNames []string
	}{
		// pass
		{`forward . ` + resolv, false, "", []string{"10.10.255.252:53", "10.10.255.253:53"}},
		// fail
		{`forward . /dev/null`, true, "no nameservers", nil},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
			continue
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr {
			for j, n := range test.expectedNames {
				addr := f.proxies[j].addr
				if n != addr {
					t.Errorf("Test %d, expected %q, got %q", j, n, addr)
				}
			}
		}
		if test.shouldErr {
			continue
		}
		for _, p := range f.proxies {
			p.health.Check(p) // this should almost always err, we don't care it shouldn't crash
		}
	}
}

func TestSetupMaxConcurrent(t *testing.T) {
	tests := []struct {
		input       string
		shouldErr   bool
		expectedVal int64
		expectedErr string
	}{
		// positive
		{"forward . 127.0.0.1 {\nmax_concurrent 1000\n}\n", false, 1000, ""},
		// negative
		{"forward . 127.0.0.1 {\nmax_concurrent many\n}\n", true, 0, "invalid"},
		{"forward . 127.0.0.1 {\nmax_concurrent -4\n}\n", true, 0, "negative"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}

		if !test.shouldErr && f.maxConcurrent != test.expectedVal {
			t.Errorf("Test %d: expected: %d, got: %d", i, test.expectedVal, f.maxConcurrent)
		}
	}
}

func TestSetupHealthCheck(t *testing.T) {
	tests := []struct {
		input       string
		shouldErr   bool
		expectedVal bool
		expectedErr string
	}{
		// positive
		{"forward . 127.0.0.1\n", false, true, ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s\n}\n", false, true, ""},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s no_rec\n}\n", false, false, ""},
		// negative
		{"forward . 127.0.0.1 {\nhealth_check no_rec\n}\n", true, true, "time: invalid duration"},
		{"forward . 127.0.0.1 {\nhealth_check 0.5s rec\n}\n", true, true, "health_check: unknown option rec"},
	}

	for i, test := range tests {
		c := caddy.NewTestController("dns", test.input)
		f, err := parseForward(c)

		if test.shouldErr && err == nil {
			t.Errorf("Test %d: expected error but found %s for input %s", i, err, test.input)
		}

		if err != nil {
			if !test.shouldErr {
				t.Errorf("Test %d: expected no error but found one for input %s, got: %v", i, test.input, err)
			}

			if !strings.Contains(err.Error(), test.expectedErr) {
				t.Errorf("Test %d: expected error to contain: %v, found error: %v, input: %s", i, test.expectedErr, err, test.input)
			}
		}
		if !test.shouldErr && (f.opts.hcRecursionDesired != test.expectedVal || f.proxies[0].health.GetRecursionDesired() != test.expectedVal) {
			t.Errorf("Test %d: expected: %t, got: %d", i, test.expectedVal, f.maxConcurrent)
		}
	}
}
