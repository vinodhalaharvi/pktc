; A device serving many remotes and every VNI.
(configure
  (ipv4 :src "192.168.1.10" :dst *
    (udp :dst-port 4789
      (vxlan :vni *
        (ethernet
          (ipv4 :src "10.100.0.1/24"))))))
