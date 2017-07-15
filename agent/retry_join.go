package agent

import (
	"fmt"
	"strings"
	"time"

	discover "github.com/hashicorp/go-discover"
)

// RetryJoin is used to handle retrying a join until it succeeds or all
// retries are exhausted.
func (a *Agent) retryJoin() {
	cfg := a.config
	if len(cfg.RetryJoin) == 0 {
		return
	}

	// split retry join addresses from go-discover statements
	var addrs []string
	var disco string
	for _, addr := range cfg.RetryJoin {
		if strings.Contains(addr, "provider=") {
			disco = addr
			continue
		}
		addrs = append(addrs, addr)
	}

	a.logger.Printf("[INFO] agent: Joining cluster...")
	attempt := 0
	for {
		var servers []string
		var err error
		if disco != "" {
			servers, err = discover.Addrs(disco, a.logger)
			if err != nil {
				a.logger.Printf("[ERR] agent: %s", err)
			}
			a.logger.Printf("[ERR] agent: Discovered servers: %v", servers)
		}

		servers = append(servers, addrs...)
		if len(servers) == 0 {
			err = fmt.Errorf("No servers to join")
		} else {
			n, err := a.JoinLAN(servers)
			if err == nil {
				a.logger.Printf("[INFO] agent: Join completed. Synced with %d initial agents", n)
				return
			}
		}

		attempt++
		if cfg.RetryMaxAttempts > 0 && attempt > cfg.RetryMaxAttempts {
			a.retryJoinCh <- fmt.Errorf("agent: max join retry exhausted, exiting")
			return
		}

		a.logger.Printf("[WARN] agent: Join failed: %v, retrying in %v", err, cfg.RetryInterval)
		time.Sleep(cfg.RetryInterval)
	}
}

// RetryJoinWan is used to handle retrying a join -wan until it succeeds or all
// retries are exhausted.
func (a *Agent) retryJoinWan() {
	cfg := a.config

	if len(cfg.RetryJoinWan) == 0 {
		return
	}

	a.logger.Printf("[INFO] agent: Joining WAN cluster...")

	attempt := 0
	for {
		n, err := a.JoinWAN(cfg.RetryJoinWan)
		if err == nil {
			a.logger.Printf("[INFO] agent: Join -wan completed. Synced with %d initial agents", n)
			return
		}

		attempt++
		if cfg.RetryMaxAttemptsWan > 0 && attempt > cfg.RetryMaxAttemptsWan {
			a.retryJoinCh <- fmt.Errorf("agent: max join -wan retry exhausted, exiting")
			return
		}

		a.logger.Printf("[WARN] agent: Join -wan failed: %v, retrying in %v", err, cfg.RetryIntervalWan)
		time.Sleep(cfg.RetryIntervalWan)
	}
}
