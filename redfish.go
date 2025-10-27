package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ---------- Redfish minimal client ----------

type rfCollection struct {
	Members []struct {
		OID string `json:"@odata.id"`
	} `json:"Members"`
}

type rfEthernetInterface struct {
	ID               string `json:"Id"`
	Name             string `json:"Name"`
	InterfaceEnabled *bool  `json:"InterfaceEnabled"`
	MACAddress       string `json:"MACAddress"`
	UefiDevicePath   string `json:"UefiDevicePath"`
	IPv4Addresses    []struct {
		Address string `json:"Address"`
		Origin  string `json:"AddressOrigin"`
	} `json:"IPv4Addresses"`
}

type rfClient struct {
	base   string
	client *http.Client
	user   string
	pass   string
}

func newRFClient(host, user, pass string, insecure bool, timeout time.Duration) *rfClient {
	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &rfClient{
		base:   "https://" + host + "/redfish/v1",
		client: &http.Client{Timeout: timeout, Transport: tr},
		user:   user,
		pass:   pass,
	}
}

func (c *rfClient) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("redfish %s: %s: %s", path, resp.Status, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func (c *rfClient) firstSystemPath(ctx context.Context) (string, error) {
	var coll rfCollection
	if err := c.get(ctx, "/Systems", &coll); err != nil {
		return "", err
	}
	if len(coll.Members) == 0 {
		return "", errors.New("no systems reported by BMC")
	}
	oid := coll.Members[0].OID
	if !strings.HasPrefix(oid, c.base) {
		return oid, nil
	}
	return strings.TrimPrefix(oid, c.base), nil
}

func (c *rfClient) listEthernetInterfaces(ctx context.Context, sysPath string) ([]rfEthernetInterface, error) {
	var coll rfCollection
	if err := c.get(ctx, sysPath+"/EthernetInterfaces", &coll); err != nil {
		return nil, err
	}
	var out []rfEthernetInterface
	for _, m := range coll.Members {
		var nic rfEthernetInterface
		path := m.OID
		if strings.HasPrefix(path, c.base) {
			path = strings.TrimPrefix(path, c.base)
		}
		if err := c.get(ctx, path, &nic); err != nil {
			return nil, err
		}
		out = append(out, nic)
	}
	return out, nil
}

// Heuristic: treat interface as bootable if:
// - UEFI device path hints (pxe/ipv4/ipv6/mac())
// - any IPv4 address origin is DHCP (common during PXE)
// - fallback: enabled with a MAC
func isBootable(n rfEthernetInterface) bool {
	uefi := strings.ToLower(n.UefiDevicePath)
	if strings.Contains(uefi, "pxe") || strings.Contains(uefi, "ipv4") || strings.Contains(uefi, "ipv6") || strings.Contains(uefi, "mac(") {
		return true
	}
	for _, a := range n.IPv4Addresses {
		if strings.EqualFold(a.Origin, "dhcp") {
			return true
		}
	}
	if n.MACAddress != "" && (n.InterfaceEnabled == nil || *n.InterfaceEnabled) {
		return true
	}
	return false
}
