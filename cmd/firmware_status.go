// SPDX-FileCopyrightText: 2025 OpenCHAMI Contributors
//
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bootstrap/internal/inventory"
	"bootstrap/internal/redfish"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	// reuse firmware flags (made persistent)
	fwStatusInterval time.Duration
)

var firmwareStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Query BMC firmware versions and in-progress updates",
	RunE: func(cmd *cobra.Command, args []string) error { // nolint:revive
		user := os.Getenv("REDFISH_USER")
		pass := os.Getenv("REDFISH_PASSWORD")
		if user == "" || pass == "" {
			return errors.New("REDFISH_USER and REDFISH_PASSWORD env vars are required")
		}

		// Determine hosts to target (reuse logic from firmware.go)
		hosts := []string{}
		if strings.TrimSpace(fwHostsCSV) != "" {
			for _, h := range strings.Split(fwHostsCSV, ",") {
				h = strings.TrimSpace(h)
				if h != "" {
					hosts = append(hosts, h)
				}
			}
		} else {
			raw, err := os.ReadFile(fwFile)
			if err != nil {
				return err
			}
			var doc inventory.FileFormat
			if err := yaml.Unmarshal(raw, &doc); err != nil {
				return err
			}
			if len(doc.BMCs) == 0 {
				return fmt.Errorf("input must contain non-empty bmcs[]")
			}
			for _, b := range doc.BMCs {
				host := b.IP
				if host == "" {
					host = b.Xname
				}
				hosts = append(hosts, host)
			}
		}

		if len(hosts) == 0 {
			return fmt.Errorf("no hosts to query")
		}

		// Determine targets
		targets := fwTargets
		if len(targets) == 0 {
			var err error
			targets, err = defaultTargets("bmc")
			if err != nil {
				return err
			}
		}

		// Results aggregation
		var mu sync.Mutex
		versionCounts := map[string]int{}
		inProgress := int32(0)
		errorsList := map[string]string{}

		sem := make(chan struct{}, max(1, fwBatchSize))
		var wg sync.WaitGroup
		for _, host := range hosts {
			wg.Add(1)
			h := host
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				ctx := cmd.Context()
				if fwTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, fwTimeout)
					defer cancel()
				}

				// Query first target (for summary)
				var ver string
				var anyInProgress bool
				for _, target := range targets {
					inv, err := redfish.GetFirmwareInventory(ctx, h, user, pass, fwInsecure, fwTimeout, target)
					if err != nil {
						// record error but continue
						mu.Lock()
						errorsList[h] = err.Error()
						mu.Unlock()
						continue
					}
					if ver == "" {
						ver = inv.Version
					}
					st := strings.ToLower(inv.State)
					if st != "" && st != "enabled" && st != "ok" {
						anyInProgress = true
					}
					for _, c := range inv.Conditions {
						m := strings.ToLower(c.Message)
						if strings.Contains(m, "updat") || strings.Contains(m, "in progress") || strings.Contains(m, "install") || strings.Contains(m, "running") {
							anyInProgress = true
						}
						if c.Severity == "Warning" || c.Severity == "Critical" {
							anyInProgress = true
						}
					}
				}

				if ver == "" {
					ver = "(unknown)"
				}

				mu.Lock()
				versionCounts[ver]++
				mu.Unlock()

				if anyInProgress {
					atomic.AddInt32(&inProgress, 1)
				}
			}()
		}
		wg.Wait()

		// Print summary
		fmt.Println("Firmware status summary:")
		fmt.Printf("  Total hosts: %d\n", len(hosts))
		fmt.Printf("  In-progress updates: %d\n", atomic.LoadInt32(&inProgress))
		fmt.Println("  Versions:")
		for v, c := range versionCounts {
			fmt.Printf("    %s: %d\n", v, c)
		}
		if len(errorsList) > 0 {
			fmt.Println("  Errors:")
			for h, e := range errorsList {
				fmt.Printf("    %s: %s\n", h, e)
			}
		}

		return nil
	},
}

func init() {
	firmwareCmd.AddCommand(firmwareStatusCmd)
	firmwareStatusCmd.Flags().DurationVar(&fwStatusInterval, "interval", 5*time.Second, "poll interval (not used in single-run summary, reserved for future watch command)")
}
