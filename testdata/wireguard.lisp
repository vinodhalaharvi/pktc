; WireGuard: encrypted IP over UDP/IP.
; The peer is named, not keyed. Key material is machine state and must
; never appear in a packet description.
(configure
  (ipv4 :src "198.51.100.10" :dst "198.51.100.20"
    (udp :src-port 51820 :dst-port 51820
      (wireguard :peer "peer-b"
        (ipv4 :src "10.44.0.1/24")))))
