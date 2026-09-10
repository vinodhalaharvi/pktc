; :dst * — one device, many peers, learned rather than configured.
(configure
  (ipv4 :src "192.168.1.10" :dst *
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24"))))))
