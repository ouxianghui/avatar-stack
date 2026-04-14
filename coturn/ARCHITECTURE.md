# Module Architecture: coturn

## Scope

Provides TURN/STUN service for WebRTC clients behind NAT/firewall.

## Responsibilities

- relay candidate allocation for clients that cannot use direct P2P path
- improve WHIP/WHEP connection success in real networks

## Operational Notes

- Local dev can use non-TLS TURN on 3478.
- Production should configure:
  - public `external-ip`
  - TLS listener (5349)
  - strong credentials/secret rotation
  - relay port range firewall rules

