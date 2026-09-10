; A tunnel inside a tunnel.
;
; Read it outside-in. The outer VXLAN runs between the two underlay
; addresses. The middle layer carries a prefix, which is the address the
; outer device needs before the inner tunnel can stand on it — and
; because that layer names both ends, the mirror derives the far side of
; it without being told. Only the innermost address has to be supplied.
;
; Two tiles cover this. There is no combined tile: the outer tile's
; device simply becomes the inner tile's underlay.

(configure
  (ipv4 :src "192.168.1.10" :dst "192.168.1.20"
    (udp :dst-port 4789
      (vxlan :vni 100
        (ethernet
          (ipv4 :src "10.100.0.1/24" :dst "10.100.0.2"
            (udp :dst-port 4789
              (vxlan :vni 200
                (ethernet
                  (ipv4 :src "10.200.0.1/24"))))))))))
