Monitoring ElasticMQ With Zabbix
===

Environment Variables
---

| Variable                           | Type    | Description                                             |
| ---------------------------------- | ------  | ---------------------------------------------           |
| `QUEUE_ENDPOINT`                   | String  | eg: `http://localhost:9324`                             |
| `ZABBIX_HOST`                      | String  | eg: `172.17.0.1`                                        |
| `ZABBIX_PORT`                      | Integer | eg: `10051`, default: `10051`                           |
| `ZABBIX_AUTO_DISCOVERY_KEY_NAME`   | String  | default: `elasticmq.queue.discovery`                    |
| `ZABBIX_ITEM_KEY_NAME`             | String  | default: `elasticmq.queue`                              |
| `INTERVAL`                         | Integer | Specify the number of seconds. eg: `300` default: `300` |

Setup
---

### Zabbix

First, Create discovery rule.

```
name: Elasticmq discovery
type: Zabbix Trapper
key:  elasticmq.queue.discovery
```

Next, create an item prototype in the discovery rule.

```
name: Monitoring of {#QUEUE} - {#ITEM}
type: Zabbix Trapper
key:  elasticmq.queue[{#QUEUE},{#ITEM}]
data type: integer
```

### Launch App

```sh
$ docker run --rm -e ZABBIX_HOST=localhost QUEUE_ENDPOINT=http://localhost:9324 flum1025/zabbix_elasticmq
```
