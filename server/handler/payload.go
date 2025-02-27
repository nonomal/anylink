package handler

import (
	"github.com/bjdgyc/anylink/base"
	"github.com/bjdgyc/anylink/dbdata"
	"github.com/bjdgyc/anylink/sessdata"
	"github.com/songgao/water/waterutil"
)

func payloadIn(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	if pl.LType == sessdata.LTypeIPData && pl.PType == 0x00 {
		// 进行Acl规则判断
		check := checkLinkAcl(cSess.Group, pl)
		if !check {
			// 校验不通过直接丢弃
			return false
		}
	}

	closed := false
	select {
	case cSess.PayloadIn <- pl:
	case <-cSess.CloseChan:
		closed = true
	}

	return closed
}

func putPayloadInBefore(cSess *sessdata.ConnSession, pl *sessdata.Payload) {
	// 异步审计日志
	if base.Cfg.AuditInterval >= 0 {
		auditPayload.Add(cSess.Username, pl)
		return
	}
	putPayload(pl)
}

func payloadOut(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	dSess := cSess.GetDtlsSession()
	if dSess == nil {
		return payloadOutCstp(cSess, pl)
	} else {
		return payloadOutDtls(cSess, dSess, pl)
	}
}

func payloadOutCstp(cSess *sessdata.ConnSession, pl *sessdata.Payload) bool {
	closed := false

	select {
	case cSess.PayloadOutCstp <- pl:
	case <-cSess.CloseChan:
		closed = true
	}

	return closed
}

func payloadOutDtls(cSess *sessdata.ConnSession, dSess *sessdata.DtlsSession, pl *sessdata.Payload) bool {
	select {
	case cSess.PayloadOutDtls <- pl:
	case <-dSess.CloseChan:
	}

	return false
}

// Acl规则校验
func checkLinkAcl(group *dbdata.Group, pl *sessdata.Payload) bool {
	if pl.LType == sessdata.LTypeIPData && pl.PType == 0x00 && len(group.LinkAcl) > 0 {
	} else {
		return true
	}

	ipDst := waterutil.IPv4Destination(pl.Data)
	ipPort := waterutil.IPv4DestinationPort(pl.Data)
	ipProto := waterutil.IPv4Protocol(pl.Data)
	// fmt.Println("sent:", ip_dst, ip_port)

	// 优先放行dns端口
	for _, v := range group.ClientDns {
		if v.Val == ipDst.String() && ipPort == 53 {
			return true
		}
	}

	for _, v := range group.LinkAcl {
		// 放行允许ip的ping
		// if v.Ports == nil || len(v.Ports) == 0 {
		// 	//单端口历史数据兼容
		// 	port := uint16(v.Port.(float64))
		// 	if port == ipPort || port == 0 || ipProto == waterutil.ICMP {
		// 		if v.Action == dbdata.Allow {
		// 			return true
		// 		} else {
		// 			return false
		// 		}
		// 	}
		// } else {

		// 先判断协议
		// 兼容旧数据 v.Protocol == ""
		if v.Protocol == "" || v.Protocol == dbdata.ALL || v.IpProto == ipProto {
			// 循环判断ip和端口
			if v.IpNet.Contains(ipDst) {
				// icmp 不判断端口
				if ipProto == waterutil.ICMP {
					if v.Action == dbdata.Allow {
						return true
					} else {
						return false
					}
				}

				if dbdata.ContainsInPorts(v.Ports, ipPort) || dbdata.ContainsInPorts(v.Ports, 0) {
					if v.Action == dbdata.Allow {
						// log.Println(dbdata.Allow, v.Ports)
						return true
					} else {
						// log.Println(dbdata.Deny, v.Ports)
						return false
					}
				}
			}
		}
	}

	return false
}
