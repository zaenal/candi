package env

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golangid/candi/candihelper"
	"github.com/golangid/candi/candishared"
	"github.com/joho/godotenv"
)

// Env model
type Env struct {
	RootApp, ServiceName string
	BuildNumber          string
	// Env on application
	Environment       string
	LoadConfigTimeout time.Duration

	useSQL, useMongo, useRedis, useRSAKey bool
	UseSharedListener                     bool

	// UseREST env
	UseREST bool
	// UseGraphQL env
	UseGraphQL bool
	// UseGRPC env
	UseGRPC bool
	// UseKafkaConsumer env
	UseKafkaConsumer bool
	// UseCronScheduler env
	UseCronScheduler bool
	// UseRedisSubscriber env
	UseRedisSubscriber bool
	// UseTaskQueueWorker env
	UseTaskQueueWorker bool
	// UsePostgresListenerWorker env
	UsePostgresListenerWorker bool
	// UseRabbitMQWorker env
	UseRabbitMQWorker bool

	DebugMode bool

	HTTPRootPath                string
	GraphQLDisableIntrospection bool

	// HTTPPort config
	HTTPPort uint16
	// GRPCPort Config
	GRPCPort uint16
	// TaskQueueDashboardPort Config
	TaskQueueDashboardPort uint16
	// TaskQueueDashboardMaxClientSubscribers Config
	TaskQueueDashboardMaxClientSubscribers int

	// BasicAuthUsername config
	BasicAuthUsername string
	// BasicAuthPassword config
	BasicAuthPassword string

	// JaegerTracingHost env
	JaegerTracingHost string

	// Jaeger max packet size in bytes (used by tracer clients)
	JaegerMaxPacketSize int

	// Broker environment
	Kafka struct {
		Brokers       []string
		ClientVersion string
		ClientID      string
		ConsumerGroup string
	}
	RabbitMQ struct {
		Broker        string
		ConsumerGroup string
		ExchangeName  string
	}

	// MaxGoroutines env for goroutine semaphore
	MaxGoroutines int

	// Database environment
	DbMongoWriteHost, DbMongoReadHost string
	DbSQLWriteDSN, DbSQLReadDSN       string
	DbRedisReadDSN, DbRedisWriteDSN   string

	// CORS Environment
	CORSAllowOrigins, CORSAllowMethods, CORSAllowHeaders []string
	CORSAllowCredential                                  bool

	StartAt string
}

var env Env

// BaseEnv get global basic environment
func BaseEnv() Env {
	return env
}

// SetEnv set env for mocking data env
func SetEnv(newEnv Env) {
	env = newEnv
}

// Load environment
func Load(serviceName string) {
	env.ServiceName = serviceName

	// load main .env and additional .env in app
	err := godotenv.Load(os.Getenv(candihelper.WORKDIR) + ".env")
	if err != nil {
		log.Printf("Warning: load env, %v", err)
	}

	mErrs := candishared.NewMultiError()

	// ------------------------------------
	parseAppConfig()
	env.BuildNumber = os.Getenv("BUILD_NUMBER")

	// LoadConfigTimeout
	if v, ok := os.LookupEnv("LOAD_CONFIG_TIMEOUT"); ok && strings.TrimSpace(v) != "" {
		if env.LoadConfigTimeout, err = time.ParseDuration(v); err != nil {
			env.LoadConfigTimeout = 10 * time.Second // fallback
		}
	} else {
		env.LoadConfigTimeout = 10 * time.Second
	}

	// ------------------------------------
	// parse Jaeger max packet size (bytes) with default e.g. 65000
	if v, ok := os.LookupEnv("JAEGER_MAX_PACKET_SIZE"); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			env.JaegerMaxPacketSize = n
		} else {
			env.JaegerMaxPacketSize = 65000
		}
	} else {
		env.JaegerMaxPacketSize = 65000
	}

	// ------------------------------------
	isServerActive := env.UseREST || env.UseGraphQL || env.UseGRPC
	if isServerActive {
		// HTTP_PORT default 8080
		httpPortStr, ok := os.LookupEnv("HTTP_PORT")
		if !ok || strings.TrimSpace(httpPortStr) == "" {
			httpPortStr = "8080"
		}
		httpPort, perr := strconv.Atoi(httpPortStr)
		if perr != nil || httpPort <= 0 || httpPort > 65535 {
			mErrs.Append("HTTP_PORT", errors.New("missing or invalid value for HTTP_PORT environment"))
		}
		env.HTTPPort = uint16(httpPort)

		// GRPC_PORT default 50051
		grpcPortStr, ok := os.LookupEnv("GRPC_PORT")
		if !ok || strings.TrimSpace(grpcPortStr) == "" {
			grpcPortStr = "50051"
		}
		grpcPort, gerr := strconv.Atoi(grpcPortStr)
		if gerr != nil || grpcPort <= 0 || grpcPort > 65535 {
			// don't fail hard, just default
			grpcPort = 50051
		}
		env.GRPCPort = uint16(grpcPort)

		env.UseSharedListener = parseBool("USE_SHARED_LISTENER")
		if env.UseSharedListener && env.HTTPPort == 0 {
			mErrs.Append("USE_SHARED_LISTENER", errors.New("missing or invalid value for HTTP_PORT environment"))
		}
	}

	// Task queue dashboard port
	if env.UseTaskQueueWorker {
		taskQueueDashboardPort, ok := os.LookupEnv("TASK_QUEUE_DASHBOARD_PORT")
		if !ok || strings.TrimSpace(taskQueueDashboardPort) == "" {
			taskQueueDashboardPort = "8080"
		}
		port, err := strconv.Atoi(taskQueueDashboardPort)
		if err != nil || port <= 0 || port > 65535 {
			mErrs.Append("TASK_QUEUE_DASHBOARD_PORT", errors.New("TASK_QUEUE_DASHBOARD_PORT environment must in integer format"))
		}
		env.TaskQueueDashboardPort = uint16(port)
		env.TaskQueueDashboardMaxClientSubscribers, _ = strconv.Atoi(os.Getenv("TASK_QUEUE_DASHBOARD_MAX_CLIENT"))
		if env.TaskQueueDashboardMaxClientSubscribers <= 0 || env.TaskQueueDashboardMaxClientSubscribers > 10 {
			env.TaskQueueDashboardMaxClientSubscribers = 10 // default
		}
	}

	// ------------------------------------
	env.Environment = os.Getenv("ENVIRONMENT")
	env.DebugMode = true // default true
	if v, ok := os.LookupEnv("DEBUG_MODE"); ok && strings.TrimSpace(v) != "" {
		env.DebugMode, _ = strconv.ParseBool(v)
	}

	env.GraphQLDisableIntrospection = parseBool("GRAPHQL_DISABLE_INTROSPECTION")
	env.HTTPRootPath = os.Getenv("HTTP_ROOT_PATH")
	env.BasicAuthUsername = os.Getenv("BASIC_AUTH_USERNAME")
	env.BasicAuthPassword = os.Getenv("BASIC_AUTH_PASS")

	env.JaegerTracingHost = os.Getenv("JAEGER_TRACING_HOST")

	// kafka environment
	parseBrokerEnv(mErrs)

	maxGoroutines, err := strconv.Atoi(os.Getenv("MAX_GOROUTINES"))
	if err != nil || maxGoroutines <= 0 {
		maxGoroutines = 10
	}
	env.MaxGoroutines = maxGoroutines

	// Parse database environment
	parseDatabaseEnv()

	// Parse CORS environment
	parseCorsEnv()

	env.StartAt = time.Now().Format(time.RFC3339)

	if mErrs.HasError() {
		panic("Basic environment error: \n" + mErrs.Error())
	}
}

func parseAppConfig() {

	useREST, ok := os.LookupEnv("USE_REST")
	if !ok {
		flag.BoolVar(&env.UseREST, "USE_REST", false, "USE REST")
	} else {
		env.UseREST, _ = strconv.ParseBool(useREST)
	}

	useGraphQL, ok := os.LookupEnv("USE_GRAPHQL")
	if !ok {
		flag.BoolVar(&env.UseGraphQL, "USE_GRAPHQL", false, "USE GRAPHQL")
	} else {
		env.UseGraphQL, _ = strconv.ParseBool(useGraphQL)
	}

	useGRPC, ok := os.LookupEnv("USE_GRPC")
	if !ok {
		flag.BoolVar(&env.UseGRPC, "USE_GRPC", false, "USE GRPC")
	} else {
		env.UseGRPC, _ = strconv.ParseBool(useGRPC)
	}

	useKafkaConsumer, ok := os.LookupEnv("USE_KAFKA_CONSUMER")
	if !ok {
		flag.BoolVar(&env.UseKafkaConsumer, "USE_KAFKA_CONSUMER", false, "USE KAFKA CONSUMER")
	} else {
		env.UseKafkaConsumer, _ = strconv.ParseBool(useKafkaConsumer)
	}

	useCronScheduler, ok := os.LookupEnv("USE_CRON_SCHEDULER")
	if !ok {
		flag.BoolVar(&env.UseCronScheduler, "USE_CRON_SCHEDULER", false, "USE CRON SCHEDULER")
	} else {
		env.UseCronScheduler, _ = strconv.ParseBool(useCronScheduler)
	}

	useRedisSubs, ok := os.LookupEnv("USE_REDIS_SUBSCRIBER")
	if !ok {
		flag.BoolVar(&env.UseRedisSubscriber, "USE_REDIS_SUBSCRIBER", false, "USE REDIS SUBSCRIBER")
	} else {
		env.UseRedisSubscriber, _ = strconv.ParseBool(useRedisSubs)
	}

	useTaskQueue, ok := os.LookupEnv("USE_TASK_QUEUE_WORKER")
	if !ok {
		flag.BoolVar(&env.UseTaskQueueWorker, "USE_TASK_QUEUE_WORKER", false, "USE TASK QUEUE WORKER")
	} else {
		env.UseTaskQueueWorker, _ = strconv.ParseBool(useTaskQueue)
	}
	usePostgresListener, ok := os.LookupEnv("USE_POSTGRES_LISTENER_WORKER")
	if !ok {
		flag.BoolVar(&env.UsePostgresListenerWorker, "USE_POSTGRES_LISTENER_WORKER", false, "USE POSTGRES LISTENER WORKER")
	} else {
		env.UsePostgresListenerWorker, _ = strconv.ParseBool(usePostgresListener)
	}
	useRabbitMQWorker, ok := os.LookupEnv("USE_RABBITMQ_CONSUMER")
	if !ok {
		flag.BoolVar(&env.UseRabbitMQWorker, "USE_RABBITMQ_CONSUMER", false, "USE RABBIT MQ CONSUMER")
	} else {
		env.UseRabbitMQWorker, _ = strconv.ParseBool(useRabbitMQWorker)
	}

	flag.Usage = func() {
		fmt.Println("	-USE_REST :=> Activate REST Server")
		fmt.Println("	-USE_GRPC :=> Activate GRPC Server")
		fmt.Println("	-USE_GRAPHQL :=> Activate GraphQL Server")
		fmt.Println("	-USE_KAFKA_CONSUMER :=> Activate Kafka Consumer Worker")
		fmt.Println("	-USE_CRON_SCHEDULER :=> Activate Cron Scheduler Worker")
		fmt.Println("	-USE_REDIS_SUBSCRIBER :=> Activate Redis Subscriber Worker")
		fmt.Println("	-USE_TASK_QUEUE_WORKER :=> Activate Task Queue Worker")
		fmt.Println("	-USE_POSTGRES_LISTENER_WORKER :=> Activate Postgres Event Worker")
		fmt.Println("	-USE_RABBITMQ_CONSUMER :=> Activate Rabbit MQ Consumer")
	}
	flag.Parse()
}

func parseBrokerEnv(mErrs candishared.MultiError) {
	kafkaBrokerEnv := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	if kafkaBrokerEnv == "" {
		env.Kafka.Brokers = []string{}
	} else {
		// trim spaces for each broker
		parts := strings.Split(kafkaBrokerEnv, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		env.Kafka.Brokers = parts
	}
	env.Kafka.ClientID = strings.TrimSpace(os.Getenv("KAFKA_CLIENT_ID"))
	env.Kafka.ClientVersion = strings.TrimSpace(os.Getenv("KAFKA_CLIENT_VERSION"))
	if env.UseKafkaConsumer {
		if len(env.Kafka.Brokers) == 0 {
			mErrs.Append("KAFKA_BROKERS", errors.New("kafka consumer is active, missing KAFKA_BROKERS environment"))
		}

		var ok bool
		env.Kafka.ConsumerGroup, ok = os.LookupEnv("KAFKA_CONSUMER_GROUP")
		if !ok || strings.TrimSpace(env.Kafka.ConsumerGroup) == "" {
			mErrs.Append("KAFKA_CONSUMER_GROUP", errors.New("kafka consumer is active, missing KAFKA_CONSUMER_GROUP environment"))
		}
	}
	env.RabbitMQ.Broker = strings.TrimSpace(os.Getenv("RABBITMQ_BROKER"))
	env.RabbitMQ.ConsumerGroup = strings.TrimSpace(os.Getenv("RABBITMQ_CONSUMER_GROUP"))
	env.RabbitMQ.ExchangeName = strings.TrimSpace(os.Getenv("RABBITMQ_EXCHANGE_NAME"))
}

func parseDatabaseEnv() {
	env.DbMongoWriteHost = strings.TrimSpace(os.Getenv("MONGODB_HOST_WRITE"))
	env.DbMongoReadHost = strings.TrimSpace(os.Getenv("MONGODB_HOST_READ"))

	env.DbSQLReadDSN = strings.TrimSpace(os.Getenv("SQL_DB_READ_DSN"))
	env.DbSQLWriteDSN = strings.TrimSpace(os.Getenv("SQL_DB_WRITE_DSN"))

	env.DbRedisReadDSN = strings.TrimSpace(os.Getenv("REDIS_READ_DSN"))
	env.DbRedisWriteDSN = strings.TrimSpace(os.Getenv("REDIS_WRITE_DSN"))
}

func parseCorsEnv() {
	CORSAllowOrigins := strings.TrimSpace(os.Getenv("CORS_ALLOW_ORIGINS"))
	if CORSAllowOrigins == "" {
		env.CORSAllowOrigins = []string{"*"}
	} else {
		parts := strings.Split(CORSAllowOrigins, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		env.CORSAllowOrigins = parts
	}
	CORSAllowMethods := strings.TrimSpace(os.Getenv("CORS_ALLOW_METHODS"))
	if CORSAllowMethods == "" {
		env.CORSAllowMethods = []string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPut,
			http.MethodPatch,
			http.MethodPost,
			http.MethodDelete,
		}
	} else {
		parts := strings.Split(CORSAllowMethods, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		env.CORSAllowMethods = parts
	}
	CORSAllowHeaders := strings.TrimSpace(os.Getenv("CORS_ALLOW_HEADERS"))
	if CORSAllowHeaders != "" {
		parts := strings.Split(CORSAllowHeaders, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		env.CORSAllowHeaders = parts
	}
	env.CORSAllowCredential, _ = strconv.ParseBool(os.Getenv("CORS_ALLOW_CREDENTIAL"))
}

func parseBool(envName string) bool {
	b, _ := strconv.ParseBool(os.Getenv(envName))
	return b
}
