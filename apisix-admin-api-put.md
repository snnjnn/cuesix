### Request Methods

| Method | Request URI                      | Request Body | Description                                                                                                                   |
| ------ | -------------------------------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| GET    | /apisix/admin/routes             | NULL         | Fetches a list of all configured Routes.                                                                                 |
| GET    | /apisix/admin/routes/{id}        | NULL         | Fetches specified Route by id.                                                                                                |
| PUT    | /apisix/admin/routes/{id}        | {...}        | Creates a Route with the specified id.                                                                                            |
| POST   | /apisix/admin/routes             | {...}        | Creates a Route and assigns a random id.                                                                                            |
| DELETE | /apisix/admin/routes/{id}        | NULL         | Removes the Route with the specified id.                                                                                      |
| PATCH  | /apisix/admin/routes/{id}        | {...}        | Updates the selected attributes of the specified, existing Route. To delete an attribute, set value of attribute set to null. |
| PATCH  | /apisix/admin/routes/{id}/{path} | {...}        | Updates the attribute specified in the path. The values of other attributes remain unchanged.                                 |

### URI Request Parameters

| parameter | Required | Type      | Description                                         | Example |
| --------- | -------- | --------- | --------------------------------------------------- | ------- |
| ttl       | False    | Auxiliary | Request expires after the specified target seconds. | ttl=1   |

### Request Body Parameters

| Parameter        | Required                                 | Type        | Description                                                                                                                                                                                                                                                                                    | Example                                              |
--
### Request Methods

| Method | Request URI                        | Request Body | Description                                                                                                                     |
| ------ | ---------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| GET    | /apisix/admin/services             | NULL         | Fetches a list of available Services.                                                                                           |
| GET    | /apisix/admin/services/{id}        | NULL         | Fetches specified Service by id.                                                                                                |
| PUT    | /apisix/admin/services/{id}        | {...}        | Creates a Service with the specified id.                                                                                            |
| POST   | /apisix/admin/services             | {...}        | Creates a Service and assigns a random id.                                                                                            |
| DELETE | /apisix/admin/services/{id}        | NULL         | Removes the Service with the specified id.                                                                                      |
| PATCH  | /apisix/admin/services/{id}        | {...}        | Updates the selected attributes of the specified, existing Service. To delete an attribute, set value of attribute set to null. |
| PATCH  | /apisix/admin/services/{id}/{path} | {...}        | Updates the attribute specified in the path. The values of other attributes remain unchanged.                                   |

### Request Body Parameters

| Parameter        | Required | Type        | Description                                                                                                        | Example                                          |
| ---------------- | -------- | ----------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| plugins          | False    | Plugin      | Plugins that are executed during the request/response cycle. See [Plugin](terminology/plugin.md) for more. |                                                  |
| upstream         | False    | Upstream    | Configuration of the [Upstream](./terminology/upstream.md).                                                |                                                  |
| upstream_id      | False    | Upstream    | Id of the [Upstream](terminology/upstream.md) service.                                                     |                                                  |
| name             | False    | Auxiliary   | Identifier for the Service.                                                                                        | service-xxxx                                     |
| desc             | False    | Auxiliary   | Description of usage scenarios.                                                                                    | service xxxx                                     |
--
### Request Methods

| Method | Request URI                        | Request Body | Description                                       |
| ------ | ---------------------------------- | ------------ | ------------------------------------------------- |
| GET    | /apisix/admin/consumers            | NULL         | Fetches a list of all Consumers.                  |
| GET    | /apisix/admin/consumers/{username} | NULL         | Fetches specified Consumer by username.           |
| PUT    | /apisix/admin/consumers            | {...}        | Create new Consumer.                              |
| DELETE | /apisix/admin/consumers/{username} | NULL         | Removes the Consumer with the specified username. |

### Request Body Parameters

| Parameter   | Required | Type        | Description                                                                                                        | Example                                          |
| ----------- | -------- | ----------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| username    | True     | Name        | Name of the Consumer.                                                                                              |                                                  |
| group_id    | False    | Name        | Group of the Consumer.                                                                                              |                                                  |
| plugins     | False    | Plugin      | Plugins that are executed during the request/response cycle. See [Plugin](terminology/plugin.md) for more. |                                                  |
| desc        | False    | Auxiliary   | Description of usage scenarios.                                                                                    | customer xxxx                                    |
| labels      | False    | Match Rules | Attributes of the Consumer specified as key-value pairs.                                                           | {"version":"v2","build":"16","env":"production"} |

Example Configuration:

--
### Request Methods

| Method | Request URI                        | Request Body | Description                                    |
| ------ |----------------------------------------------------------------|--------------|------------------------------------------------|
| GET    | /apisix/admin/consumers/{username}/credentials                 | NUll         | Fetches list of all credentials of the Consumer |
| GET    | /apisix/admin/consumers/{username}/credentials/{credential_id} | NUll         | Fetches the Credential by `credential_id`      |
| PUT    | /apisix/admin/consumers/{username}/credentials/{credential_id} | {...}        | Create or update a Creddential                 |
| DELETE | /apisix/admin/consumers/{username}/credentials/{credential_id} | NUll         | Delete the Credential                          |

### Request Body Parameters

| Parameter   | Required | Type        | Description                                                | Example                                         |
| ----------- |-----| ------- |------------------------------------------------------------|-------------------------------------------------|
| plugins     | False    | Plugin      | Auth plugins configuration.                                |                                                 |
| name        | False    | Auxiliary   | Identifier for the Credential.                             | credential_primary                              |
| desc        | False    | Auxiliary   | Description of usage scenarios.                            | credential xxxx                                 |
| labels      | False    | Match Rules | Attributes of the Credential specified as key-value pairs. | {"version":"v2","build":"16","env":"production"} |

Example Configuration:

```shell
--
### Request Methods

| Method | Request URI                         | Request Body | Description                                                                                                                      |
| ------ | ----------------------------------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| GET    | /apisix/admin/upstreams             | NULL         | Fetch a list of all configured Upstreams.                                                                                        |
| GET    | /apisix/admin/upstreams/{id}        | NULL         | Fetches specified Upstream by id.                                                                                                |
| PUT    | /apisix/admin/upstreams/{id}        | {...}        | Creates an Upstream with the specified id.                                                                                           |
| POST   | /apisix/admin/upstreams             | {...}        | Creates an Upstream and assigns a random id.                                                                                           |
| DELETE | /apisix/admin/upstreams/{id}        | NULL         | Removes the Upstream with the specified id.                                                                                      |
| PATCH  | /apisix/admin/upstreams/{id}        | {...}        | Updates the selected attributes of the specified, existing Upstream. To delete an attribute, set value of attribute set to null. |
| PATCH  | /apisix/admin/upstreams/{id}/{path} | {...}        | Updates the attribute specified in the path. The values of other attributes remain unchanged.                                    |

### Request Body Parameters

In addition to the equalization algorithm selections, Upstream also supports passive health check and retry for the upstream. See the table below for more details:

| Parameter                   | Required                                                         | Type                          | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Example                                                                                                                                    |
|-----------------------------|------------------------------------------------------------------|-------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| type                        | False                                                            | Enumeration                   | Load balancing algorithm to be used, and the default value is `roundrobin`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |                                                                                                                                            |
| nodes                       | True, can't be used with `service_name`                          | Node                          | IP addresses (with optional ports) of the Upstream nodes represented as a hash table or an array. In the hash table, the key is the IP address and the value is the weight of the node for the load balancing algorithm. For hash table case, if the key is IPv6 address with port, then the IPv6 address must be quoted with square brackets. In the array, each item is a hash table with keys `host`, `weight`, and the optional `port` and `priority` (defaults to `0`). Nodes with lower priority are used only when all nodes with a higher priority are tried and are unavailable. Empty nodes are treated as placeholders and clients trying to access this Upstream will receive a 502 response.                                                   | `192.168.1.100:80`, `[::1]:80`                                                                                                             |
| service_name                | True, can't be used with `nodes`                                 | String                        | Service name used for [service discovery](discovery.md).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | `a-bootiful-client`                                                                                                                        |
--
### Request Methods

| Method | Request URI            | Request Body | Description                                     |
| ------ | ---------------------- | ------------ | ----------------------------------------------- |
| GET    | /apisix/admin/ssls      | NULL         | Fetches a list of all configured SSL resources. |
| GET    | /apisix/admin/ssls/{id} | NULL         | Fetch specified resource by id.                 |
| PUT    | /apisix/admin/ssls/{id} | {...}        | Creates a resource with the specified id.           |
| POST   | /apisix/admin/ssls      | {...}        | Creates a resource and assigns a random id.           |
| DELETE | /apisix/admin/ssls/{id} | NULL         | Removes the resource with the specified id.     |

### Request Body Parameters

| Parameter    | Required | Type                     | Description                                                                                                    | Example                                          |
| ------------ | -------- | ------------------------ | -------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ |
| cert         | True     | Certificate              | HTTPS certificate. This field supports saving the value in Secret Manager using the [APISIX Secret](./terminology/secret.md) resource.                                                                                             |                                                  |
| key          | True     | Private key              | HTTPS private key. This field supports saving the value in Secret Manager using the [APISIX Secret](./terminology/secret.md) resource.                                                                                             |                                                  |
| certs        | False    | An array of certificates | Used for configuring multiple certificates for the same domain excluding the one provided in the `cert` field. This field supports saving the value in Secret Manager using the [APISIX Secret](./terminology/secret.md) resource.  |                                                  |
| keys         | False    | An array of private keys | Private keys to pair with the `certs`. This field supports saving the value in Secret Manager using the [APISIX Secret](./terminology/secret.md) resource.                                                                   |                                                  |
| client.ca    | False    | Certificate              | Sets the CA certificate that verifies the client. Requires OpenResty 1.19+.                                    |                                                  |
| client.depth | False    | Certificate              | Sets the verification depth in client certificate chains. Defaults to 1. Requires OpenResty 1.19+.             |                                                  |
| client.skip_mtls_uri_regex | False    | An array of regular expressions, in PCRE format              | Used to match URI, if matched, this request bypasses the client certificate checking, i.e. skip the MTLS.             | ["/hello[0-9]+", "/foobar"]                                                |
--
### Request Methods

| Method | Request URI                            | Request Body | Description                                                                                                                         |
| ------ | -------------------------------------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| GET    | /apisix/admin/global_rules             | NULL         | Fetches a list of all Global Rules.                                                                                                 |
| GET    | /apisix/admin/global_rules/{id}        | NULL         | Fetches specified Global Rule by id.                                                                                                |
| PUT    | /apisix/admin/global_rules/{id}        | {...}        | Creates a Global Rule with the specified id.                                                                                        |
| DELETE | /apisix/admin/global_rules/{id}        | NULL         | Removes the Global Rule with the specified id.                                                                                      |
| PATCH  | /apisix/admin/global_rules/{id}        | {...}        | Updates the selected attributes of the specified, existing Global Rule. To delete an attribute, set value of attribute set to null. |
| PATCH  | /apisix/admin/global_rules/{id}/{path} | {...}        | Updates the attribute specified in the path. The values of other attributes remain unchanged.                                       |

### Request Body Parameters

| Parameter   | Required | Description                                                                                                        | Example    |
| ----------- | -------- | ------------------------------------------------------------------------------------------------------------------ | ---------- |
| plugins     | True     | Plugins that are executed during the request/response cycle. See [Plugin](terminology/plugin.md) for more. |            |

## Consumer group

Group of Plugins which can be reused across Consumers.

--
### Request Methods

| Method | Request URI                              | Request Body | Description                                                                                                                           |
| ------ | ---------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | /apisix/admin/consumer_groups             | NULL         | Fetches a list of all Consumer groups.                                                                                                 |
| GET    | /apisix/admin/consumer_groups/{id}        | NULL         | Fetches specified Consumer group by id.                                                                                                |
| PUT    | /apisix/admin/consumer_groups/{id}        | {...}        | Creates a new Consumer group with the specified id.                                                                                    |
| DELETE | /apisix/admin/consumer_groups/{id}        | NULL         | Removes the Consumer group with the specified id.                                                                                      |
| PATCH  | /apisix/admin/consumer_groups/{id}        | {...}        | Updates the selected attributes of the specified, existing Consumer group. To delete an attribute, set value of attribute set to null. |
| PATCH  | /apisix/admin/consumer_groups/{id}/{path} | {...}        | Updates the attribute specified in the path. The values of other attributes remain unchanged.                                         |

### Request Body Parameters

| Parameter   | Required | Description                                                                                                        | Example                                          |
| ----------- | -------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| plugins     | True     | Plugins that are executed during the request/response cycle. See [Plugin](terminology/plugin.md) for more. |                                                  |
| name        | False    | Identifier for the consumer group.                                                                                 | premium-tier                            |
| desc        | False    | Description of usage scenarios.                                                                                    | customer xxxx                                    |
| labels      | False    | Attributes of the Consumer group specified as key-value pairs.                                                      | {"version":"v2","build":"16","env":"production"} |

## Plugin config
--
### Request Methods

| Method | Request URI                              | Request Body | Description                                                                                                                           |
| ------ | ---------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------- |
| GET    | /apisix/admin/plugin_configs             | NULL         | Fetches a list of all Plugin configs.                                                                                                 |
| GET    | /apisix/admin/plugin_configs/{id}        | NULL         | Fetches specified Plugin config by id.                                                                                                |
| PUT    | /apisix/admin/plugin_configs/{id}        | {...}        | Creates a new Plugin config with the specified id.                                                                                    |
| DELETE | /apisix/admin/plugin_configs/{id}        | NULL         | Removes the Plugin config with the specified id.                                                                                      |
| PATCH  | /apisix/admin/plugin_configs/{id}        | {...}        | Updates the selected attributes of the specified, existing Plugin config. To delete an attribute, set value of attribute set to null. |
| PATCH  | /apisix/admin/plugin_configs/{id}/{path} | {...}        | Updates the attribute specified in the path. The values of other attributes remain unchanged.                                         |

### Request Body Parameters

| Parameter   | Required | Description                                                                                                        | Example                                          |
| ----------- | -------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| plugins     | True     | Plugins that are executed during the request/response cycle. See [Plugin](terminology/plugin.md) for more. |                                                  |
| desc        | False    | Description of usage scenarios.                                                                                    | customer xxxx                                    |
| labels      | False    | Attributes of the Plugin config specified as key-value pairs.                                                      | {"version":"v2","build":"16","env":"production"} |

## Plugin Metadata

--
### Request Methods

| Method | Request URI                                 | Request Body | Description                                                     |
| ------ | ------------------------------------------- | ------------ | --------------------------------------------------------------- |
| GET    | /apisix/admin/plugin_metadata               | NULL         | Fetches a list of all Plugin metadata.                          |
| GET    | /apisix/admin/plugin_metadata/{plugin_name} | NULL         | Fetches the metadata of the specified Plugin by `plugin_name`.  |
| PUT    | /apisix/admin/plugin_metadata/{plugin_name} | {...}        | Creates metadata for the Plugin specified by the `plugin_name`. |
| DELETE | /apisix/admin/plugin_metadata/{plugin_name} | NULL         | Removes metadata for the Plugin specified by the `plugin_name`. |

### Request Body Parameters

A JSON object defined according to the `metadata_schema` of the Plugin ({plugin_name}).

Example Configuration:

```shell
curl http://127.0.0.1:9180/apisix/admin/plugin_metadata/example-plugin  \
-H "X-API-KEY: $admin_key" -i -X PUT -d '
{
    "skey": "val",
    "ikey": 1
--
### Request Methods

| Method | Request URI                         | Request Body | Description                                    |
| ------ | ----------------------------------- | ------------ | ---------------------------------------------- |
| GET    | /apisix/admin/plugins/list          | NULL         | Fetches a list of all Plugins.                 |
| GET    | /apisix/admin/plugins/{plugin_name} | NULL         | Fetches the specified Plugin by `plugin_name`. |
| GET         | /apisix/admin/plugins?all=true      | NULL         | Get all properties of all plugins. |
| GET         | /apisix/admin/plugins?all=true&subsystem=stream| NULL | Gets properties of all Stream plugins.|
| GET    | /apisix/admin/plugins?all=true&subsystem=http | NULL | Gets properties of all HTTP plugins. |
| PUT    | /apisix/admin/plugins/reload       | NULL         | Reloads the plugin according to the changes made in code |
| GET    | apisix/admin/plugins/{plugin_name}?subsystem=stream | NULL | Gets properties of a specified plugin if it is supported in Stream/L4 subsystem. |
| GET    | apisix/admin/plugins/{plugin_name}?subsystem=http   | NULL | Gets properties of a specified plugin if it is supported in HTTP/L7 subsystem. |

:::caution

The interface of getting properties of all plugins via `/apisix/admin/plugins?all=true` will be deprecated soon.

:::

### Request Body Parameters

--
### Request Methods

| Method | Request URI                      | Request Body | Description                                     |
| ------ | -------------------------------- | ------------ | ----------------------------------------------- |
| GET    | /apisix/admin/stream_routes      | NULL         | Fetches a list of all configured Stream Routes. |
| GET    | /apisix/admin/stream_routes/{id} | NULL         | Fetches specified Stream Route by id.           |
| PUT    | /apisix/admin/stream_routes/{id} | {...}        | Creates a Stream Route with the specified id.       |
| POST   | /apisix/admin/stream_routes      | {...}        | Creates a Stream Route and assigns a random id.       |
| DELETE | /apisix/admin/stream_routes/{id} | NULL         | Removes the Stream Route with the specified id. |

### Request Body Parameters

| Parameter   | Required | Type     | Description                                                         | Example                       |
| ----------- | -------- | -------- | ------------------------------------------------------------------- | ----------------------------- |
| name        | False    | Auxiliary | Identifier for the Stream Route.                                   | postgres-proxy                |
| desc        | False    | Auxiliary | Description of usage scenarios.                                    | proxy endpoint for postgresql |
| labels      | False    | Match Rules | Attributes of the Proto specified as key-value pairs.    | {"version":"17","service":"user","env":"production"}     |
| upstream    | False    | Upstream | Configuration of the [Upstream](./terminology/upstream.md). |                               |
| upstream_id | False    | Upstream | Id of the [Upstream](terminology/upstream.md) service.      |                               |
| service_id  | False    | String   | Id of the [Service](terminology/service.md) service.        |                               |
| remote_addr | False    | IPv4, IPv4 CIDR, IPv6  | Filters Upstream forwards by matching with client IP.               | "127.0.0.1" or "127.0.0.1/32" or "::1" |
--
### Request Methods

| Method | Request URI                        | Request Body | Description                                       |
| ------ | ---------------------------------- | ------------ | ------------------------------------------------- |
| GET    | /apisix/admin/secrets            | NULL         | Fetches a list of all secrets.                  |
| GET    | /apisix/admin/secrets/{manager}/{id} | NULL         | Fetches specified secrets by id.           |
| PUT    | /apisix/admin/secrets/{manager}            | {...}        | Create new secrets configuration.                              |
| DELETE | /apisix/admin/secrets/{manager}/{id} | NULL         | Removes the secrets with the specified id. |
| PATCH  | /apisix/admin/secrets/{manager}/{id}        | {...}        | Updates the selected attributes of the specified, existing secrets. To delete an attribute, set value of attribute set to null. |
| PATCH  | /apisix/admin/secrets/{manager}/{id}/{path} | {...}        | Updates the attribute specified in the path. The values of other attributes remain unchanged.                                 |

### Request Body Parameters

#### When Secret Manager is Vault

| Parameter   | Required | Type        | Description                                                                                                        | Example                                          |
| ----------- | -------- | ----------- | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------ |
| uri    | True     | URI        | URI of the vault server.                                                                                              |                                                  |
| prefix    | True    | string        | key prefix
| token     | True    | string      | vault token. |                                                  |
| namespace | False   | string       | Vault namespace, no default value | `admin` |
--
### Request Methods

| Method | Request URI                      | Request Body | Description                                     |
| ------ | -------------------------------- | ------------ | ----------------------------------------------- |
| GET    | /apisix/admin/protos      | NULL         | List all Protos.  |
| GET    | /apisix/admin/protos/{id} | NULL         | Get a Proto by id.     |
| PUT    | /apisix/admin/protos/{id} | {...}        | Create or update a Proto with the given id.        |
| POST   | /apisix/admin/protos      | {...}        | Create a Proto with a random id.         |
| DELETE | /apisix/admin/protos/{id} | NULL         | Delete Proto by id.                 |

### Request Body Parameters

| Parameter | Required | Type      | Description                          | Example                       |
|-----------|----------|-----------|--------------------------------------| ----------------------------- |
| content   | True     | String    | Content of `.proto` or `.pb` files   | See [here](./plugins/grpc-transcode.md#enabling-the-plugin)         |
| name      | False    | Auxiliary | Identifier for the Protobuf definition. | user-proto                    |
| desc      | False    | Auxiliary | Description of usage scenarios.      | protobuf for user service     |
| labels    | False    | Match Rules | Attributes of the Proto specified as key-value pairs. | {"version":"v2","service":"user","env":"production"}     |

## Schema validation

--
### Request Methods

| Method | Request URI                      | Request Body | Description                                     |
| ------ | -------------------------------- | ------------ | ----------------------------------------------- |
| POST   | /apisix/admin/schema/validate/{resource}      | {..resource conf..}        | Validate the resource configuration against corresponding schema.         |

### Request Body Parameters

* 200: validate ok.
* 400: validate failed, with error as response body in JSON format.

Example:

```bash
curl http://127.0.0.1:9180/apisix/admin/schema/validate/routes \
    -H "X-API-KEY: $admin_key" -X POST -i -d '{
    "uri": 1980,
    "upstream": {
        "scheme": "https",
        "type": "roundrobin",
        "nodes": {
