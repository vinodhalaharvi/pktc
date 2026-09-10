; A multicast outer destination is not a peer: it is the group every
; peer joins. The kernel parameter changes with it.
(configure
  (ipv4 :src "192.168.1.10" :dst "239.1.1.1"
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24"))))))
