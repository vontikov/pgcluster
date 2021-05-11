package integration

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
	flag "github.com/spf13/pflag"
	"github.com/vontikov/pgcluster/internal/logging"
	"github.com/vontikov/pgcluster/internal/util"
)

const configPath = "../assets/test/cfg/"
const defaultConfigFile = "config-stoa.yaml"

var configFile = flag.String("config", defaultConfigFile, "test configuration")

var opts = godog.Options{
	Output: colors.Colored(os.Stdout),
	Format: "pretty",
	Paths:  []string{"../assets/test/features"},
}

func init() {
	godog.BindCommandLineFlags("godog.", &opts)
}

func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {})
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	cfg, err := NewConfig(configPath + *configFile)
	util.PanicOnError(err)

	logging.SetLevel(cfg.Logging.Level)

	h, err := NewTestHandle(cfg)
	util.PanicOnError(err)

	ctx.BeforeScenario(func(*godog.Scenario) { util.PanicOnError(h.Start()) })
	ctx.AfterScenario(func(sc *godog.Scenario, err error) { util.PanicOnError(h.Stop()) })

	ctx.Step(`^SUT$`, h.systemUnderTest)
	ctx.Step(`^the master is populated$`, h.theDatabasePopulated)
	ctx.Step(`^replicas are synchronized$`, h.allReplicasSynchronized)

	ctx.Step(`^master is down$`, h.masterIsDown)
	ctx.Step(`^a replica should be promoted$`, h.aReplicaShouldBePromotedToTheNewMaster)
	ctx.Step(`^another replica should follow the new master$`, h.anotherReplicaShouldFollowTheNewMaster)
	ctx.Step(`^the new master should be populated with new data$`, h.newMasterPopulated)

	ctx.Step(`^all up again$`, h.allUpAgain)
	ctx.Step(`^replicas should contain new data$`, h.replicasShouldContainNewData)
}

func TestMain(m *testing.M) {
	flag.Parse()

	status := godog.TestSuite{
		Name:                 "godog",
		TestSuiteInitializer: InitializeTestSuite,
		ScenarioInitializer:  InitializeScenario,
		Options:              &opts,
	}.Run()

	if st := m.Run(); st > status {
		status = st
	}
}
