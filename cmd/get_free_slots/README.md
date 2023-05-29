
`curl -v http://10.0.0.1:8881/metrics/datacenter/all_slots?action=list&format=zabbix`
`curl -v http://10.0.0.1:8881/metrics/datacenter/all_slots?action=get_total_number&format=zabbix&id=<datacenter id>`
`curl -v http://10.0.0.1:8881/metrics/datacenter/all_slots?action=get_active_number&format=zabbix&id=<datacenter id>`


`curl -v http://10.0.0.1:8881/metrics/datacenter/free_slots?action=list`
`curl -v http://10.0.0.1:8881/metrics/datacenter/free_slots?action=get_total_number&format=zabbix&id=<datacenter id>`
`curl -v http://10.0.0.1:8881/metrics/datacenter/free_slots?action=get_active_number&format=zabbix&id=<datacenter id>`
