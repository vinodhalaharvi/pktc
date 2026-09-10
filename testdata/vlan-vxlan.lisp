; A realistic chain: a tagged interface carrying a VXLAN tunnel.
; Two tiles, and the tagged device is what the tunnel is built on.
(configure
  (ethernet
    (vlan :id 100
      (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
        (udp :dst-port 4789
          (vxlan :vni 200
            (ethernet
              (ipv4 :src "10.200.0.1/24"))))))))
