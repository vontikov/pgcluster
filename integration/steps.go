package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/messages-go/v10"
	"github.com/vontikov/pgcluster/internal/metric"
	"golang.org/x/sync/errgroup"
)

const busyLoopSleepDuration = 1000 * time.Millisecond
const serviceStateTimeout = 30 * time.Second

type (
	table    = messages.PickleStepArgument_PickleTable
	tableRow = messages.PickleStepArgument_PickleTable_PickleTableRow
)

func (h *TestHandle) systemUnderTest(t *table) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), serviceStateTimeout)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	for _, row := range t.Rows {
		r := row
		g.Go(func() error {
			service := r.Cells[0].Value
			expectedState := r.Cells[1].Value
			return h.checkServiceState(ctx, service, expectedState)
		})
	}
	if err = g.Wait(); err != nil {
		return
	}

	h.saveInitialState(t)
	return
}

func (h *TestHandle) theDatabasePopulated() error {
	const sql = `
CREATE TABLE countries (
	country varchar,
	alpha2  char(2),
	alpha3  char(3),
	numeric char(3)
);
COPY countries FROM '/data/countries.csv' WITH (FORMAT csv, DELIMITER ';');
`
	master, err := h.getMaster()
	if err != nil {
		return err
	}
	return h.populate(master, sql)
}

func (h *TestHandle) allReplicasSynchronized() error {
	const sql = `SELECT country, alpha2, alpha3, numeric FROM countries;`
	return h.compare(sql)
}

func (h *TestHandle) masterIsDown() (err error) {
	h.logger.Debug("stopping the master")

	for service, state := range h.serviceStates {
		h.logger.Trace("checking", "name", service, "state", state)
		if state == Master {
			id := h.containerIDs[service]
			err = h.dc.ContainerStop(id)
			h.serviceStates[service] = Stopped
			h.logger.Debug("master stopped", "name", service)
			return
		}
	}
	return fmt.Errorf("no master to stop: %v", h.serviceStates)
}

func (h *TestHandle) aReplicaShouldBePromotedToTheNewMaster() error {
	return h.updateClusterState()
}

func (h *TestHandle) anotherReplicaShouldFollowTheNewMaster() error {
	h.logger.Debug("server states", "states", h.serviceStates)

	var master string
	for service, state := range h.serviceStates {
		if state != Master {
			continue
		}
		master = service
		break
	}

	ctx, cancel := context.WithTimeout(context.Background(), serviceStateTimeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("new follower not found within: %v", serviceStateTimeout)
		default:
			for service, state := range h.serviceStates {
				if state == Stopped || state == Master {
					continue
				}
				h.logger.Debug("receiving master info for", "service", service)
				mi, err := h.getMasterInfo(service)
				if err != nil {
					break
				}
				h.logger.Debug("received master info for", "service", service, "info", mi)
				if mi.Host == master {
					return nil
				}
			}
			time.Sleep(busyLoopSleepDuration)
		}
	}
}

func (h *TestHandle) newMasterPopulated() error {
	master, err := h.getMaster()
	if err != nil {
		return err
	}

	const sql = `
CREATE TABLE calling_codes (
	country varchar,
	codes   varchar
);
COPY calling_codes FROM '/data/calling-codes.csv' WITH (FORMAT csv, DELIMITER ';');
`
	return h.populate(master, sql)
}

func (h *TestHandle) allUpAgain() error {
	for service := range h.serviceStates {
		state := h.serviceStates[service]
		if state == Stopped {
			id := h.containerIDs[service]
			if err := h.dc.ContainerStart(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *TestHandle) replicasShouldContainNewData() error {
	const sql = `SELECT country, codes FROM calling_codes;`
	return h.compare(sql)
}

func (h *TestHandle) checkServiceState(ctx context.Context, service serviceName, expectedState string) (err error) {
	h.logger.Debug("checking service", "name", service, "expected state", expectedState)
	var r bool
	for {
		select {
		case <-ctx.Done():
			return
		default:
			r, err = h.getRecoveryStatus(service)
			if err != nil {
				time.Sleep(busyLoopSleepDuration)
				break
			}
			if h.expectedInitialRecoveryStatus[expectedState] != r {
				return fmt.Errorf("unexpected service state: name=%s, state=%v", service, r)
			}
			return
		}
	}
}

func (h *TestHandle) saveInitialState(t *table) {
	for _, row := range t.Rows {
		service := row.Cells[0].Value
		state := row.Cells[1].Value
		h.serviceStates[service] = serviceStateFromString(state)
	}
}

func (h *TestHandle) populate(service serviceName, sql string) (err error) {
	h.logger.Debug("populating database via", "service", service)
	conn, err := h.connGet(service)
	if err != nil {
		return
	}
	defer conn.Close(context.Background())

	_, err = conn.Exec(context.Background(), sql)
	return
}

func (h *TestHandle) compare(sql string) error {
	f := func(service serviceName) ([][]string, error) {
		conn, err := h.connGet(service)
		if err != nil {
			return nil, err
		}
		defer conn.Close(context.Background())

		ctx, cancel := context.WithTimeout(context.Background(), h.queryTimeout)
		defer cancel()

		rows, err := conn.Query(ctx, sql)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var res [][]string
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				return nil, err
			}

			var r []string
			for _, v := range vals {
				r = append(r, v.(string))
			}
			res = append(res, r)
		}
		return res, nil
	}

	master, err := h.getMaster()
	if err != nil {
		return err
	}

	h.logger.Debug("reading master data", "service", master)
	masterData, err := f(master)
	if err != nil {
		return err
	}

	for service, state := range h.serviceStates {
		if state != Replica {
			continue
		}

		h.logger.Debug("reading replica data", "service", service)
		replicaData, err := f(service)
		if err != nil {
			return err
		}

		h.logger.Debug("comparing data", "service", service)
		if len(masterData) != len(replicaData) {
			return fmt.Errorf("different data set sizes")
		}

		for i := range masterData {
			e := masterData[i]
			a := replicaData[i]

			if len(e) != len(a) {
				return fmt.Errorf("different record sizes")
			}
			for j := range e {
				if e[j] != a[j] {
					return fmt.Errorf("different fields")
				}
			}
		}
	}
	return nil
}

func (h *TestHandle) updateClusterState() error {
	ctx, cancel := context.WithTimeout(context.Background(), serviceStateTimeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("master not found in: %v", serviceStateTimeout)
		default:

			var masterFound bool

			for service := range h.serviceStates {
				state := h.serviceStates[service]
				if state == Stopped {
					continue
				}
				rs, err := h.getRecoveryStatus(service)
				if err != nil {
					return err
				}
				if !rs {
					if masterFound {
						return fmt.Errorf("another master found: service=%s", service)
					}
					masterFound = true
					h.serviceStates[service] = Master
				} else {
					h.serviceStates[service] = Replica
				}
			}
			if masterFound {
				return nil
			}
			time.Sleep(busyLoopSleepDuration)
		}
	}
}

func (h *TestHandle) getMaster() (master serviceName, err error) {
	for service, state := range h.serviceStates {
		if state != Master {
			continue
		}
		master = service
		return
	}
	err = fmt.Errorf("master not found")
	return
}

func (h *TestHandle) getRecoveryStatus(service serviceName) (r bool, err error) {
	h.logger.Debug("checking recovery status", "name", service)
	var m metricValue

	if m, err = h.getMetric(service, metric.IsAlive); err != nil {
		h.logger.Trace("metric error", "name", metric.IsAlive, "message", err)
		return
	}
	if m != 1.0 {
		err = fmt.Errorf("service is not reachable: name=%s, metric=%v", service, m)
		return
	}

	if m, err = h.getMetric(service, metric.InRecovery); err != nil {
		h.logger.Trace("metric error", "name", metric.InRecovery, "message", err)
		return
	}
	switch m {
	case 1.0:
		r = true
	case 0.0:
		r = false
	default:
		err = fmt.Errorf("unknown recovery status: name=%s", service)
	}
	return
}
