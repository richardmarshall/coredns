package autopath

import "github.com/miekg/dns"

// Writer implements a ResponseWriter that also does the following:
// * reverts question section of a packet to its original state.
//   This is done to avoid the 'Question section mismatch:' error in client.
// * Defers write to the client if the response code is NXDOMAIN.  This is needed
//   to enable further search path probing if a search was not successful.
// * Allow forced write to client regardless of response code.  This is needed to
//   write the packet to the client if the final search in the path fails to
//   produce results.
// * Overwrites response code with Writer.Rcode if the response code
//   is NXDOMAIN (NameError).  This is needed to support the AutoPath.OnNXDOMAIN
//   function, which returns a NOERROR to client instead of NXDOMAIN if the final
//   search in the path fails to produce results.
type Writer struct {
	dns.ResponseWriter
	original dns.Question
	Rcode    int
	Sent     bool
}

// AutoPath enables server side search path lookups for pods.
type AutoPath struct {
	NDots          int
	ResolvConfFile string
	HostSearchPath []string
	OnNXDOMAIN     int
}

// NewWriter returns a pointer to a new Writer
func NewWriter(w dns.ResponseWriter, r *dns.Msg) *Writer {
	return &Writer{
		ResponseWriter: w,
		original:       r.Question[0],
		Rcode:          dns.RcodeSuccess,
	}
}

// WriteMsg writes to client, unless response will be NXDOMAIN.
func (apw *Writer) WriteMsg(res *dns.Msg) error {
	return apw.overrideMsg(res, false)
}

// ForceWriteMsg forces the write to client regardless of response code.
func (apw *Writer) ForceWriteMsg(res *dns.Msg) error {
	return apw.overrideMsg(res, true)
}

// overrideMsg overrides rcode, reverts question, adds CNAME, and calls the
// underlying ResponseWriter's WriteMsg method unless the write is deferred,
// or force = true.
func (apw *Writer) overrideMsg(res *dns.Msg, force bool) error {
	if res.Rcode == dns.RcodeNameError {
		res.Rcode = apw.Rcode
	}
	if res.Rcode != dns.RcodeSuccess && !force {
		return nil
	}
	for _, a := range res.Answer {
		if apw.original.Name == a.Header().Name {
			continue
		}
		res.Answer = append(res.Answer, nil)
		copy(res.Answer[1:], res.Answer)
		res.Answer[0] = CNAME(apw.original.Name, a.Header().Name, a.Header().Ttl)
	}
	res.Question[0] = apw.original
	apw.Sent = true
	return apw.ResponseWriter.WriteMsg(res)
}

// Write is a wrapper that records the size of the message that gets written.
func (apw *Writer) Write(buf []byte) (int, error) {
	n, err := apw.ResponseWriter.Write(buf)
	return n, err
}

// Hijack implements dns.Hijacker. It simply wraps the underlying
// ResponseWriter's Hijack method if there is one, or returns an error.
func (apw *Writer) Hijack() {
	apw.ResponseWriter.Hijack()
	return
}
