package node

import (
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/robfig/cron"
)

type CronManager struct {
	p2pm *P2PManager
	cfgm *utils.ConfigManager
	lm   *utils.LogsManager
}

func NewCronManager(p2pm *P2PManager) *CronManager {
	return &CronManager{
		p2pm: p2pm,
		cfgm: utils.NewConfigManager(""),
		lm:   utils.NewLogsManager(),
	}
}

func (cm *CronManager) JobQueue() error {
	configs, err := cm.cfgm.ReadConfigs()
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return err
	}

	jm := NewJobManager(cm.p2pm)
	c := cron.New()
	err = c.AddFunc(configs["process_job_queue"], jm.ProcessQueue)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return err
	}
	err = c.AddFunc(configs["request_job_status_update"], jm.RequestWorkflowJobsStatusUpdates)
	if err != nil {
		cm.lm.Log("error", err.Error(), "cron")
		return err
	}
	c.Start()

	return nil
}
