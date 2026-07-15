package templates

type TemplateData struct {
	APIVIP    string
	ProxyPort string
	NFTRules  string
}

const NFTRuleTemplate = `
table inet crtmgr_proxy_table
delete table inet crtmgr_proxy_table
table inet crtmgr_proxy_table {
    chain crtmgr_proxy_PREROUTING {
        type nat hook prerouting priority 0;
        # Redirect to proxy port
        ip daddr {{ .APIVIP }} tcp dport 80 redirect to {{ .ProxyPort }}
    }
}`

const MachineConfigurationManifest = `
apiVersion: operator.openshift.io/v1
kind: MachineConfiguration
metadata:
  name: cluster
spec:
  nodeDisruptionPolicy:
    files:
    - actions:
      - restart:
          serviceName: nftables.service
        type: Restart
      path: /etc/sysconfig/nftables.conf
    units:
    - actions:
      - type: DaemonReload
      - type: Reload
        reload:
          serviceName: nftables.service
      name: nftables.service`

const MachineConfigTemplate = `
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: master
  name: 98-nftables-crtmgr-proxy
spec:
  config:
    ignition:
      version: 3.4.0
    storage:
      files:
        - contents:
            source: data:text/plain;charset=utf-8;base64,{{ .NFTRules }}
          mode: 384
          overwrite: true
          path: /etc/sysconfig/nftables.conf
    systemd:
      units:
        - contents: |
            [Unit]
            Description=Netfilter Tables
            Documentation=man:nft(8)
            Wants=network-pre.target
            Before=network-pre.target
            [Service]
            Type=oneshot
            ProtectSystem=full
            ProtectHome=true
            ExecStart=/sbin/nft -f /etc/sysconfig/nftables.conf
            ExecReload=/sbin/nft -f /etc/sysconfig/nftables.conf
            ExecStop=/sbin/nft 'add table inet crtmgr_proxy_table; delete table inet crtmgr_proxy_table'
            RemainAfterExit=yes
            [Install]
            WantedBy=multi-user.target
          enabled: true
          name: nftables.service`
