; VXLAN: Ethernet over UDP/IP.
(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24"))))))
