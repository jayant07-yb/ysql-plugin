package ysql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/vault/helper/testhelpers/docker"

	dbplugin "github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	dbtesting "github.com/hashicorp/vault/sdk/database/dbplugin/v5/testing"
	"github.com/hashicorp/vault/sdk/helper/template"
	"github.com/stretchr/testify/require"
)

func PrepareTestContainer(t *testing.T, version string) (func(), string) {

	if version == "" {
		version = "latest"
	}
	runner, err := docker.NewServiceRunner(docker.RunOptions{
		ImageRepo:     "yugabytedb/yugabyte",
		ContainerName: "yugabyte-vault-test",
		ImageTag:      version,
		Env:           []string{"POSTGRES_PASSWORD=secret", "POSTGRES_DB=database"},
		Ports:         []string{"5433/tcp", "7000/tcp", "9000/tcp", "9042/tcp"},
	})
	if err != nil {
		t.Fatalf("Could not start docker Yugabyte: %s", err)
	}

	svc, err := runner.StartService(context.Background(), connectYsql)
	if err != nil {
		t.Fatalf("Could not start docker Yugabyte: %s", err)
	}

	return svc.Cleanup, svc.Config.URL().String()
}

func connectYsql(ctx context.Context, host string, port int) (docker.ServiceConfig, error) {
	u := url.URL{
		Scheme:   "yugabyte",
		User:     url.UserPassword("yugabyte", "secret"),
		Host:     fmt.Sprintf("%s:%d", host, port),
		Path:     "yugabytedb",
		RawQuery: "sslmode=disable",
	}

	db, err := sql.Open("postgres", u.String())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		return nil, err
	}
	return docker.NewServiceURL(u), nil
}

func getYsql(t *testing.T, options map[string]interface{}) (*ysql, func()) {
	cleanup, connURL := PrepareTestContainer(t, "latest")

	connectionDetails := map[string]interface{}{
		"connection_url": connURL,
	}
	fmt.Printf("The link:: %s\n\n\n\n", connURL)
	for k, v := range options {
		connectionDetails[k] = v
	}

	req := dbplugin.InitializeRequest{
		Config:           connectionDetails,
		VerifyConnection: true,
	}

	db := new()
	dbtesting.AssertInitialize(t, db, req)

	if !db.Initialized {
		t.Fatal("Database should be initialized")
	}
	return db, cleanup
}

func TestYsql_Initialize(t *testing.T) {
	db, cleanup := getYsql(t, map[string]interface{}{
		"max_open_connections": 5,
	})
	defer cleanup()

	if err := db.Close(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestYsql_InitializeWithStringVals(t *testing.T) {
	db, cleanup := getYsql(t, map[string]interface{}{
		"max_open_connections": "5",
	})
	defer cleanup()

	if err := db.Close(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

type credsAssertion func(t testing.TB, connURL, username, password string)

func assertCreds(assertions ...credsAssertion) credsAssertion {
	return func(t testing.TB, connURL, username, password string) {
		t.Helper()
		for _, assertion := range assertions {
			assertion(t, connURL, username, password)
		}
	}
}

func assertUsernameRegex(rawRegex string) credsAssertion {
	return func(t testing.TB, _, username, _ string) {
		t.Helper()
		require.Regexp(t, rawRegex, username)
	}
}

func assertCredsExist(t testing.TB, connURL, username, password string) {
	t.Helper()
	err := testCredsExist(t, connURL, username, password)
	if err != nil {
		t.Fatalf("user does not exist: %s", err)
	}
}

func assertCredsDoNotExist(t testing.TB, connURL, username, password string) {
	t.Helper()
	err := testCredsExist(t, connURL, username, password)
	if err == nil {
		t.Fatalf("user should not exist but does")
	}
}

func waitUntilCredsDoNotExist(timeout time.Duration) credsAssertion {
	return func(t testing.TB, connURL, username, password string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				t.Fatalf("Timed out waiting for user %s to be deleted", username)
			case <-ticker.C:
				err := testCredsExist(t, connURL, username, password)
				if err != nil {
					// Happy path
					return
				}
			}
		}
	}
}

func assertCredsExistAfter(timeout time.Duration) credsAssertion {
	return func(t testing.TB, connURL, username, password string) {
		t.Helper()
		time.Sleep(timeout)
		assertCredsExist(t, connURL, username, password)
	}
}

func testCredsExist(t testing.TB, connURL, username, password string) error {
	t.Helper()
	// Log in with the new creds
	connURL = strings.Replace(connURL, "postgres:secret", fmt.Sprintf("%s:%s", username, password), 1)
	db, err := sql.Open("postgres", connURL)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Ping()
}

const createAdminUser = `
CREATE ROLE "{{name}}" WITH
  LOGIN
  PASSWORD '{{password}}'
  VALID UNTIL '{{expiration}}';
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO "{{name}}";
`

var newUserLargeBlockStatements = []string{
	`
DO $$
BEGIN
   IF NOT EXISTS (SELECT * FROM pg_catalog.pg_roles WHERE rolname='foo-role') THEN
      CREATE ROLE "foo-role";
      CREATE SCHEMA IF NOT EXISTS foo AUTHORIZATION "foo-role";
      ALTER ROLE "foo-role" SET search_path = foo;
      GRANT TEMPORARY ON DATABASE "postgres" TO "foo-role";
      GRANT ALL PRIVILEGES ON SCHEMA foo TO "foo-role";
      GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA foo TO "foo-role";
      GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA foo TO "foo-role";
      GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA foo TO "foo-role";
   END IF;
END
$$
`,
	`CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';`,
	`GRANT "foo-role" TO "{{name}}";`,
	`ALTER ROLE "{{name}}" SET search_path = foo;`,
	`GRANT CONNECT ON DATABASE "postgres" TO "{{name}}";`,
}

func TestContainsMultilineStatement(t *testing.T) {
	type testCase struct {
		Input    string
		Expected bool
	}

	testCases := map[string]*testCase{
		"issue 6098 repro": {
			Input:    `DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname='my_role') THEN CREATE ROLE my_role; END IF; END $$`,
			Expected: true,
		},
		"multiline with template fields": {
			Input:    `DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname="{{name}}") THEN CREATE ROLE {{name}}; END IF; END $$`,
			Expected: true,
		},
		"docs example": {
			Input: `CREATE ROLE "{{name}}" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; \
        GRANT SELECT ON ALL TABLES IN SCHEMA public TO "{{name}}";`,
			Expected: false,
		},
	}

	for tName, tCase := range testCases {
		t.Run(tName, func(t *testing.T) {
			if containsMultilineStatement(tCase.Input) != tCase.Expected {
				t.Fatalf("%q should be %t for multiline input", tCase.Input, tCase.Expected)
			}
		})
	}
}

func TestExtractQuotedStrings(t *testing.T) {
	type testCase struct {
		Input    string
		Expected []string
	}

	testCases := map[string]*testCase{
		"no quotes": {
			Input:    `Five little monkeys jumping on the bed`,
			Expected: []string{},
		},
		"two of both quote types": {
			Input:    `"Five" little 'monkeys' "jumping on" the' 'bed`,
			Expected: []string{`"Five"`, `"jumping on"`, `'monkeys'`, `' '`},
		},
		"one single quote": {
			Input:    `Five little monkeys 'jumping on the bed`,
			Expected: []string{},
		},
		"empty string": {
			Input:    ``,
			Expected: []string{},
		},
		"templated field": {
			Input:    `DO $$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname="{{name}}") THEN CREATE ROLE {{name}}; END IF; END $$`,
			Expected: []string{`"{{name}}"`},
		},
	}

	for tName, tCase := range testCases {
		t.Run(tName, func(t *testing.T) {
			results, err := extractQuotedStrings(tCase.Input)
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != len(tCase.Expected) {
				t.Fatalf("%s isn't equal to %s", results, tCase.Expected)
			}
			for i := range results {
				if results[i] != tCase.Expected[i] {
					t.Fatalf(`expected %q but received %q`, tCase.Expected, results[i])
				}
			}
		})
	}
}

func TestUsernameGeneration(t *testing.T) {
	type testCase struct {
		data          dbplugin.UsernameMetadata
		expectedRegex string
	}

	tests := map[string]testCase{
		"simple display and role names": {
			data: dbplugin.UsernameMetadata{
				DisplayName: "token",
				RoleName:    "myrole",
			},
			expectedRegex: `v-token-myrole-[a-zA-Z0-9]{20}-[0-9]{10}`,
		},
		"display name has dash": {
			data: dbplugin.UsernameMetadata{
				DisplayName: "token-foo",
				RoleName:    "myrole",
			},
			expectedRegex: `v-token-fo-myrole-[a-zA-Z0-9]{20}-[0-9]{10}`,
		},
		"display name has underscore": {
			data: dbplugin.UsernameMetadata{
				DisplayName: "token_foo",
				RoleName:    "myrole",
			},
			expectedRegex: `v-token_fo-myrole-[a-zA-Z0-9]{20}-[0-9]{10}`,
		},
		"display name has period": {
			data: dbplugin.UsernameMetadata{
				DisplayName: "token.foo",
				RoleName:    "myrole",
			},
			expectedRegex: `v-token.fo-myrole-[a-zA-Z0-9]{20}-[0-9]{10}`,
		},
		"role name has dash": {
			data: dbplugin.UsernameMetadata{
				DisplayName: "token",
				RoleName:    "myrole-foo",
			},
			expectedRegex: `v-token-myrole-f-[a-zA-Z0-9]{20}-[0-9]{10}`,
		},
		"role name has underscore": {
			data: dbplugin.UsernameMetadata{
				DisplayName: "token",
				RoleName:    "myrole_foo",
			},
			expectedRegex: `v-token-myrole_f-[a-zA-Z0-9]{20}-[0-9]{10}`,
		},
		"role name has period": {
			data: dbplugin.UsernameMetadata{
				DisplayName: "token",
				RoleName:    "myrole.foo",
			},
			expectedRegex: `v-token-myrole.f-[a-zA-Z0-9]{20}-[0-9]{10}`,
		},
	}

	for name, test := range tests {
		t.Run(fmt.Sprintf("new-%s", name), func(t *testing.T) {
			up, err := template.NewTemplate(
				template.Template(defaultUserNameTemplate),
			)
			require.NoError(t, err)

			for i := 0; i < 1000; i++ {
				username, err := up.Generate(test.data)
				require.NoError(t, err)
				require.Regexp(t, test.expectedRegex, username)
			}
		})
	}
}

func TestNewUser_CustomUsername(t *testing.T) {
	cleanup, connURL := PrepareTestContainer(t, "13.4-buster")
	defer cleanup()

	type testCase struct {
		usernameTemplate string
		newUserData      dbplugin.UsernameMetadata
		expectedRegex    string
	}

	tests := map[string]testCase{
		"default template": {
			usernameTemplate: "",
			newUserData: dbplugin.UsernameMetadata{
				DisplayName: "displayname",
				RoleName:    "longrolename",
			},
			expectedRegex: "^v-displayn-longrole-[a-zA-Z0-9]{20}-[0-9]{10}$",
		},
		"explicit default template": {
			usernameTemplate: defaultUserNameTemplate,
			newUserData: dbplugin.UsernameMetadata{
				DisplayName: "displayname",
				RoleName:    "longrolename",
			},
			expectedRegex: "^v-displayn-longrole-[a-zA-Z0-9]{20}-[0-9]{10}$",
		},
		"unique template": {
			usernameTemplate: "foo-bar",
			newUserData: dbplugin.UsernameMetadata{
				DisplayName: "displayname",
				RoleName:    "longrolename",
			},
			expectedRegex: "^foo-bar$",
		},
		"custom prefix": {
			usernameTemplate: "foobar-{{.DisplayName | truncate 8}}-{{.RoleName | truncate 8}}-{{random 20}}-{{unix_time}}",
			newUserData: dbplugin.UsernameMetadata{
				DisplayName: "displayname",
				RoleName:    "longrolename",
			},
			expectedRegex: "^foobar-displayn-longrole-[a-zA-Z0-9]{20}-[0-9]{10}$",
		},
		"totally custom template": {
			usernameTemplate: "foobar_{{random 10}}-{{.RoleName | uppercase}}.{{unix_time}}x{{.DisplayName | truncate 5}}",
			newUserData: dbplugin.UsernameMetadata{
				DisplayName: "displayname",
				RoleName:    "longrolename",
			},
			expectedRegex: `^foobar_[a-zA-Z0-9]{10}-LONGROLENAME\.[0-9]{10}xdispl$`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			initReq := dbplugin.InitializeRequest{
				Config: map[string]interface{}{
					"connection_url":    connURL,
					"username_template": test.usernameTemplate,
				},
				VerifyConnection: true,
			}

			db := new()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			_, err := db.Initialize(ctx, initReq)
			require.NoError(t, err)

			newUserReq := dbplugin.NewUserRequest{
				UsernameConfig: test.newUserData,
				Statements: dbplugin.Statements{
					Commands: []string{
						`
						CREATE ROLE "{{name}}" WITH
						  LOGIN
						  PASSWORD '{{password}}'
						  VALID UNTIL '{{expiration}}';
						GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO "{{name}}";`,
					},
				},
				Password:   "myReally-S3curePassword",
				Expiration: time.Now().Add(1 * time.Hour),
			}
			ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			newUserResp, err := db.NewUser(ctx, newUserReq)
			require.NoError(t, err)

			require.Regexp(t, test.expectedRegex, newUserResp.Username)
		})
	}
}
