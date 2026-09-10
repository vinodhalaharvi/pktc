; IP-in-IP, with traffic below the tunnel that is not configuration.
(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (ipv4 :src "10.30.0.1/30"
      (tcp :dst-port 22))))
