
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=root

export GOPATH=$HOME/go
export PATH=$PATH:$GOROOT/bin:$GOPATH/bin


go build -o /home/jayantanand/code/work/hashicorp/plugins/ysql-plugin cmd/ysql-plugin/main.go

vault server -dev -dev-root-token-id=root -dev-plugin-dir=/home/jayantanand/code/work/hashicorp/plugins


vault secrets enable database

vault write database/config/yugabytedb plugin_name=ysql-plugin  \
    host="127.0.0.1" \
    port=5433 \
    username="yugabyte" \
    password="yugabyte" \
    db="yugabyte" \
    allowed_roles="*"

vault write database/roles/my-first-role \
    db_name=yugabytedb \
    creation_statements="CREATE ROLE \"{{username}}\" WITH PASSWORD '{{password}}' NOINHERIT LOGIN; \
       GRANT ALL ON DATABASE \"yugabyte\" TO \"{{username}}\";" \
    default_ttl="1h" \
    max_ttl="24h"


vault read database/creds/my-first-role

vault lease revoke  database/creds/my-first-role/mpTUxJsYYpbtN91axfjW6cfQ
vault read database/creds/my-first-role


# V_TOKEN_MY-FIRST-ROLE_68WGYCXFWPET4SHKPVKZ_1649350992