/**
 * MIT License
 *
 * Copyright (c) 2017 - 2018 CNES
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */
package api

import (
	"errors"
	. "mal"
)

const (
	_SEND_HANDLER UOctet = iota
	_SUBMIT_HANDLER
	_REQUEST_HANDLER
	_INVOKE_HANDLER
	_PROGRESS_HANDLER
	_BROKER_HANDLER
)

type handlerDesc struct {
	handlerType UOctet
	area        UShort
	areaVersion UOctet
	service     UShort
	operation   UShort
	handler     Handler
}

type HandlerContext struct {
	Ctx      *Context
	Uri      *URI
	ch       chan *Message
	handlers map[uint64](*handlerDesc)
}

func NewHandlerContext(ctx *Context, service string) (*HandlerContext, error) {
	// TODO (AF): Verify the uri
	uri := ctx.NewURI(service)
	// TODO (AF): Fix length of channel?
	ch := make(chan *Message, 10)
	handlers := make(map[uint64](*handlerDesc))
	hctx := &HandlerContext{ctx, uri, ch, handlers}
	err := ctx.RegisterEndPoint(uri, hctx)
	if err != nil {
		return nil, err
	}
	return hctx, nil
}

func (hctx *HandlerContext) register(hdltype UOctet, area UShort, areaVersion UOctet, service UShort, operation UShort, handler Handler) error {
	key := key(area, areaVersion, service, operation)
	old := hctx.handlers[key]

	if old != nil {
		logger.Errorf("MAL handler already registered: %d", key)
		return errors.New("MAL handler already registered")
	} else {
		logger.Debugf("MAL handler registered: %d", key)
	}

	var desc = &handlerDesc{
		handlerType: hdltype,
		area:        area,
		areaVersion: areaVersion,
		service:     service,
		operation:   operation,
		handler:     handler,
	}

	hctx.handlers[key] = desc
	return nil
}

func (hctx *HandlerContext) Close() error {
	return hctx.Ctx.UnregisterEndPoint(hctx.Uri)
}

// Defines a generic root handler interface
type Handler func(*Message, Transaction) error

// ================================================================================
// SendHandler

type SendHandler func(*Message, SendTransaction) error

// TODO (AF):
//func (hctx *ProviderContext) RegisterSendHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler SendHandler) error {
func (hctx *HandlerContext) RegisterSendHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler Handler) error {
	return hctx.register(_SEND_HANDLER, area, areaVersion, service, operation, handler)
}

// ================================================================================
// SubmitHandler

type SubmitHandler func(*Message, SubmitTransaction) error

// TODO (AF):
//func (hctx *ProviderContext) RegisterSendHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler SendHandler) error {
func (hctx *HandlerContext) RegisterSubmitHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler Handler) error {
	return hctx.register(_SUBMIT_HANDLER, area, areaVersion, service, operation, handler)
}

// ================================================================================
// RequestHandler

type RequestHandler func(*Message, RequestTransaction) error

// TODO (AF):
//func (hctx *ProviderContext) RegisterRequestHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler RequestHandler) error {
func (hctx *HandlerContext) RegisterRequestHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler Handler) error {
	return hctx.register(_REQUEST_HANDLER, area, areaVersion, service, operation, handler)
}

// ================================================================================
// InvokeHandler

type InvokeHandler func(*Message, InvokeTransaction) error

// TODO (AF):
//func (hctx *ProviderContext) RegisterInvokeHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler InvokeHandler) error {
func (hctx *HandlerContext) RegisterInvokeHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler Handler) error {
	return hctx.register(_INVOKE_HANDLER, area, areaVersion, service, operation, handler)
}

// ================================================================================
// ProgressHandler

type ProgressHandler func(*Message, ProgressTransaction) error

// TODO (AF):
//func (hctx *ProviderContext) RegisterSendHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler SendHandler) error {
func (hctx *HandlerContext) RegisterProgressHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler Handler) error {
	return hctx.register(_PROGRESS_HANDLER, area, areaVersion, service, operation, handler)
}

// ================================================================================
// BrokerHandler: There is only one handler but 2 transactions type depending of the
// incoming interaction.

type BrokerHandler func(*Message, BrokerTransaction) error

// TODO (AF):
//func (hctx *ProviderContext) RegisterBrokerHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler BrokerHandler) error {
func (hctx *HandlerContext) RegisterBrokerHandler(area UShort, areaVersion UOctet, service UShort, operation UShort, handler Handler) error {
	return hctx.register(_BROKER_HANDLER, area, areaVersion, service, operation, handler)
}

// ================================================================================
// Defines Listener interface used by context to route MAL messages

func (hctx *HandlerContext) getHandler(hdltype UOctet, area UShort, areaVersion UOctet, service UShort, operation UShort) (Handler, error) {
	key := key(area, areaVersion, service, operation)

	to, ok := hctx.handlers[key]
	if ok {
		if to.handlerType == hdltype {
			return to.handler, nil
		} else {
			logger.Errorf("Bad handler type: %d should be %d", to.handlerType, hdltype)
			return nil, errors.New("Bad handler type")
		}
	} else {
		logger.Errorf("MAL handler not registered: %d", key)
		return nil, errors.New("MAL handler not registered")
	}
}

func (hctx *HandlerContext) OnMessage(msg *Message) error {
	// TODO (AF): We can use msg.InteractionType as selector
	switch msg.InteractionType {
	case MAL_INTERACTIONTYPE_SEND:
		handler, err := hctx.getHandler(_SEND_HANDLER, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation)
		if err != nil {
			return err
		}
		transaction := &SendTransactionX{TransactionX{hctx.Ctx, hctx.Uri, msg.UriFrom, msg.TransactionId, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation}}
		// TODO (AF): use a goroutine
		return handler(msg, transaction)
	case MAL_INTERACTIONTYPE_SUBMIT:
		handler, err := hctx.getHandler(_SUBMIT_HANDLER, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation)
		if err != nil {
			return err
		}
		transaction := &SubmitTransactionX{TransactionX{hctx.Ctx, hctx.Uri, msg.UriFrom, msg.TransactionId, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation}}
		// TODO (AF): use a goroutine
		return handler(msg, transaction)
	case MAL_INTERACTIONTYPE_REQUEST:
		handler, err := hctx.getHandler(_REQUEST_HANDLER, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation)
		if err != nil {
			return err
		}
		transaction := &RequestTransactionX{TransactionX{hctx.Ctx, hctx.Uri, msg.UriFrom, msg.TransactionId, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation}}
		// TODO (AF): use a goroutine
		return handler(msg, transaction)
	case MAL_INTERACTIONTYPE_INVOKE:
		handler, err := hctx.getHandler(_INVOKE_HANDLER, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation)
		if err != nil {
			return err
		}
		transaction := &InvokeTransactionX{TransactionX{hctx.Ctx, hctx.Uri, msg.UriFrom, msg.TransactionId, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation}}
		// TODO (AF): use a goroutine
		return handler(msg, transaction)
	case MAL_INTERACTIONTYPE_PROGRESS:
		handler, err := hctx.getHandler(_PROGRESS_HANDLER, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation)
		if err != nil {
			return err
		}
		transaction := &ProgressTransactionX{TransactionX{hctx.Ctx, hctx.Uri, msg.UriFrom, msg.TransactionId, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation}}
		// TODO (AF): use a goroutine
		return handler(msg, transaction)
	case MAL_INTERACTIONTYPE_PUBSUB:
		handler, err := hctx.getHandler(_BROKER_HANDLER, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation)
		if err != nil {
			return err
		}
		var transaction Transaction
		if (msg.InteractionStage == MAL_IP_STAGE_PUBSUB_PUBLISH_REGISTER) ||
			(msg.InteractionStage == MAL_IP_STAGE_PUBSUB_PUBLISH) ||
			(msg.InteractionStage == MAL_IP_STAGE_PUBSUB_PUBLISH_DEREGISTER) {
			transaction = &PublisherTransactionX{TransactionX{hctx.Ctx, hctx.Uri, msg.UriFrom, msg.TransactionId, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation}}
		} else if (msg.InteractionStage == MAL_IP_STAGE_PUBSUB_REGISTER) ||
			(msg.InteractionStage == MAL_IP_STAGE_PUBSUB_DEREGISTER) {
			transaction = &SubscriberTransactionX{TransactionX{hctx.Ctx, hctx.Uri, msg.UriFrom, msg.TransactionId, msg.ServiceArea, msg.AreaVersion, msg.Service, msg.Operation}}
		} else {
			// TODO (AF): Log an error, May be we should not return this error
			return errors.New("Bad interaction stage for PubSub")
		}
		// TODO (AF): use a goroutine
		return handler(msg, transaction)
	default:
		logger.Debugf("Cannot route message to: %s", *msg.UriTo)
	}
	return nil
}

func (hctx *HandlerContext) OnClose() error {
	logger.Infof("close EndPoint: %s", hctx.Uri)
	// TODO (AF): Close handlers ?
	//	for key, handler := range hctx.handlers {
	//		fmt.Println("close handler: ", key)
	//		err := handler.OnClose()
	//		if err != nil {
	//			// TODO (AF): print an error message
	//		}
	//	}
	return nil
}
