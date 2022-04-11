##  Completion matrix
|API/TASK|Status|
|-|-|
| Initialize the plugin|✅|
| Create User |✅ |
| Delete User|✅|
| Update User|✅|
| Make File| |
| Create User -test| |
| Delete User -test| |
| Update User -test| |
| Blog| |
| Add the smart driver's feature|   |


##  Steps to be followed to use the terminal

Admin's terminal to configure the database
```sh
#   Clone and go to the database plugin directory
$ go build -o /home/jayantanand/code/work/hashicorp/plugins/ysql-plugin cmd/ysql-plugin/main.go

$ export VAULT_ADDR="http://localhost:8200"

$ export VAULT_TOKEN="root"

```

Run the vault server
```sh
$   vault server -dev -dev-root-token-id=root -dev-plugin-dir=/home/jayantanand/code/work/hashicorp/plugins
```

Register the plugin , config the database and create the role 
```sh
#   Register the plugin
$ export SHA256=$(sha256sum /home/jayantanand/code/work/hashicorp/plugins/ysql-plugin  | cut -d' ' -f1)


$ vault secrets enable database

$ vault write sys/plugins/catalog/database/ysql-plugin \
    sha256=$SHA256 \
    command="ysql-plugin"

#   Add the database
$ vault write database/config/yugabytedb plugin_name=ysql-plugin  \
    host="127.0.0.1" \
    port=5433 \
    username="yugabyte" \
    password="yugabyte" \
    db="yugabyte" \
    allowed_roles="*"

#   Create the role
$ vault write database/roles/my-first-role \
    db_name=yugabytedb \
    creation_statements="CREATE ROLE \"{{username}}\" WITH PASSWORD '{{password}}' NOINHERIT LOGIN; \
       GRANT ALL ON DATABASE \"yugabyte\" TO \"{{username}}\";" \
    default_ttl="1h" \
    max_ttl="24h"

#   For managing the lease
$   vault lease lookup database/creds/my-first-role/MML1XWMjcJKXBlk47HHs6HrZ

$   vault lease renew  database/creds/my-first-role/MML1XWMjcJKXBlk47HHs6HrZ

$   vault lease revoke   database/creds/my-first-role/E8cCdoKTn9mvQjQAWd5aZohQ
```


-   Client/App code
Create the user 
```sh
$   vault read database/creds/my-first-role
```
docker  exec -it 7f0bbe99f83b  bash

##  Error with the revoke statement::
-   `failed to revoke lease: lease_id=database/creds/my-first-role/MML1XWMjcJKXBlk47HHs6HrZ error="failed to revoke entry: resp: (*logical.Response)(nil) err: unable to delete user: rpc error: code = Internal desc = unable to delete user: pq: role \"V_TOKEN_MY-FIRST-ROLE_HDZVDJXNAEYNDNWVW2IU_1649353280\" cannot be dropped because some objects depend on it"`

