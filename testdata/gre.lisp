; GRE: IP over GRE over IP.
(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (gre
      (ipv4 :src "10.20.0.1/30"))))
