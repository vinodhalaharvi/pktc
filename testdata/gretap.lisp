; GRETAP: Ethernet over GRE over IP.
; Same first two layers as gre.lisp; the inner layer picks the device.
(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (gre :key 500
      (ethernet
        (ipv4 :src "10.50.0.1/24")))))
