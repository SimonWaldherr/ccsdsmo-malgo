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
	"sync/atomic"
)

type OperationHandler interface {
	onMessage(msg *Message)
	onClose()
}

type OperationContext struct {
	Ctx       *Context
	Uri       *URI
	handlers  map[ULong]OperationHandler
	txcounter uint64
}

func NewOperationContext(ctx *Context, service string) (*OperationContext, error) {
	// TODO (AF): Verify the uri
	uri := ctx.NewURI(service)
	handlers := make(map[ULong]OperationHandler)
	ictx := &OperationContext{ctx, uri, handlers, 0}
	err := ctx.RegisterEndPoint(uri, ictx)
	if err != nil {
		return nil, err
	}
	return ictx, nil
}

func (ictx *OperationContext) register(tid ULong, handler OperationHandler) error {
	// TODO (AF): Synchronization
	old := ictx.handlers[tid]
	if old != nil {
		logger.Warnf("Handler already registered for this transaction: %d", tid)
		return errors.New("Handler already registered for this transaction")
	}
	ictx.handlers[tid] = handler
	return nil
}

func (ictx *OperationContext) deregister(tid ULong) error {
	// TODO (AF): Synchronization
	if ictx.handlers[tid] == nil {
		logger.Warnf("No handler registered for this transaction: %d", tid)
		return errors.New("No handler registered for this transaction")
	}
	delete(ictx.handlers, tid)
	return nil
}

func (ictx *OperationContext) TransactionId() ULong {
	return ULong(atomic.AddUint64(&ictx.txcounter, 1))
}

func (ictx *OperationContext) Close() error {
	return ictx.Ctx.UnregisterEndPoint(ictx.Uri)
}

const (
	_CREATED byte = iota
	_INITIATED
	_ACKNOWLEDGED
	_PROGRESSING
	_REGISTER_INITIATED
	_REGISTERED
	_REREGISTER_INITIATED
	_DEREGISTER_INITIATED
	_FINAL
	_CLOSED
)

type Operation interface {
	GetTid() ULong
	verify(msg *Message) bool
	finalize()
	Close() error
	Reset() error
}

type OperationX struct {
	ictx        *OperationContext
	tid         ULong
	ch          chan *Message
	urito       *URI
	area        UShort
	areaVersion UOctet
	service     UShort
	operation   UShort
	status      byte
}

// Verifies that the incoming message corresponds to the initiated operation
func (op *OperationX) verify(msg *Message) bool {
	if (msg.ServiceArea == op.area) && (msg.AreaVersion == op.areaVersion) &&
		(msg.Service == op.service) && (msg.Operation == op.operation) {
		return true
	}
	return false
}

// Finalize the operation
func (op *OperationX) finalize() {
	op.status = _FINAL
	if op.ch != nil {
		// This operation should not received anymore messages, unregisters it
		// in OperationContext
		op.ictx.deregister(op.tid)
	}
}

func (op *OperationX) GetTid() ULong {
	return op.tid
}

// Closes the operation.
// Be careful a closed operation cannot be used anymore.
func (op *OperationX) Close() error {
	if op.status == _CLOSED {
		return nil
	}
	op.status = _CLOSED
	if op.ch != nil {
		var err error = nil
		if (op.status != _CREATED) && (op.status != _FINAL) {
			err = op.ictx.deregister(op.tid)
		}
		close(op.ch)
		op.ch = nil
		return err
	}
	return nil
}

// Resets the operation for a new use, a new TransactionId is allocated.
// Be careful, the operation must be in a FINAL state
func (op *OperationX) Reset() error {
	if op.status != _FINAL {
		return errors.New("Bad operation status")
	}
	// Gets a new TransactionId for operation
	op.tid = op.ictx.TransactionId()
	op.status = _CREATED
	return nil
}

// ================================================================================
// SendOperation

type SendOperation interface {
	Operation
	Send(body []byte) error
}

type SendOperationX struct {
	OperationX
}

func (ictx *OperationContext) NewSendOperation(urito *URI, area UShort, areaVersion UOctet, service UShort, operation UShort) SendOperation {
	// Gets a new TransactionId for operation
	tid := ictx.TransactionId()
	op := &SendOperationX{OperationX: OperationX{ictx, tid, nil, urito, area, areaVersion, service, operation, _CREATED}}
	return op
}

func (op *SendOperationX) Send(body []byte) error {
	if op.status != _CREATED {
		return errors.New("Bad operation status")
	}
	op.status = _INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_SEND,
		InteractionStage: MAL_IP_STAGE_SEND,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// This operation doesn't wait any reply, so we don't need to register it.
	// Send the SEND MAL message
	err := op.ictx.Ctx.Send(msg)
	op.status = _FINAL
	return err
}

func (op *SendOperationX) onMessage(msg *Message) {
	// TODO (AF): Should never reveive messages, log an error
}

func (op *SendOperationX) onClose() {
	// TODO (AF): Should never be called, log an error
}

// ================================================================================
// SubmitOperation

type SubmitOperation interface {
	Operation
	Submit(body []byte) (*Message, error)
}

type SubmitOperationX struct {
	OperationX
}

func (ictx *OperationContext) NewSubmitOperation(urito *URI, area UShort, areaVersion UOctet, service UShort, operation UShort) SubmitOperation {
	// Gets a new TransactionId for operation
	tid := ictx.TransactionId()
	// TODO (AF): Fix length of channel
	ch := make(chan *Message, 10)
	op := &SubmitOperationX{OperationX: OperationX{ictx, tid, ch, urito, area, areaVersion, service, operation, _CREATED}}
	return op
}

func (op *SubmitOperationX) Submit(body []byte) (*Message, error) {
	if op.status != _CREATED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_SUBMIT,
		InteractionStage: MAL_IP_STAGE_SUBMIT,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Registers this Submit Operation in OperationContext
	err := op.ictx.register(op.tid, op)
	if err != nil {
		op.finalize()
		return nil, err
	}
	// Send the SUBMIT MAL message
	err = op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the SUBMIT_ACK MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Errorf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	if msg.InteractionStage != MAL_IP_STAGE_SUBMIT_ACK {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	op.finalize()
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

func (op *SubmitOperationX) onMessage(msg *Message) {
	// Verify the message: service area, version, service, operation
	if op.verify(msg) && (msg.InteractionType == MAL_INTERACTIONTYPE_SUBMIT) {
		op.ch <- msg
	} else {
		logger.Errorf("SUBMIT Operation (%s,%d) receives Bad message: %+v", *op.urito, op.tid, msg)
	}
}

func (op *SubmitOperationX) onClose() {
	// TODO (AF):
}

// ================================================================================
// RequestOperation

type RequestOperation interface {
	Operation
	Request(body []byte) (*Message, error)
}

type RequestOperationX struct {
	OperationX
}

func (ictx *OperationContext) NewRequestOperation(urito *URI, area UShort, areaVersion UOctet, service UShort, operation UShort) RequestOperation {
	// Gets a new TransactionId for operation
	tid := ictx.TransactionId()
	// TODO (AF): Fix length of channel
	ch := make(chan *Message, 10)
	op := &RequestOperationX{OperationX: OperationX{ictx, tid, ch, urito, area, areaVersion, service, operation, _CREATED}}
	return op
}

func (op *RequestOperationX) Request(body []byte) (*Message, error) {
	if op.status != _CREATED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_REQUEST,
		InteractionStage: MAL_IP_STAGE_REQUEST,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Registers this Request Operation in OperationContext
	err := op.ictx.register(op.tid, op)
	if err != nil {
		op.finalize()
		return nil, err
	}
	// Send the REQUEST MAL message
	err = op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the RESPONSE MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_REQUEST_RESPONSE {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	op.finalize()
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

func (op *RequestOperationX) onMessage(msg *Message) {
	// Verify the message: service area, version, service, operation
	if op.verify(msg) && (msg.InteractionType == MAL_INTERACTIONTYPE_REQUEST) {
		op.ch <- msg
	} else {
		logger.Errorf("REQUEST Operation (%s,%d) receives Bad message: %+v", *op.urito, op.tid, msg)
	}
}

func (op *RequestOperationX) onClose() {
	// TODO (AF):
}

// ================================================================================
// Invoke Operation

type InvokeOperation interface {
	Operation
	Invoke(body []byte) (*Message, error)
	GetResponse() (*Message, error)
}

type InvokeOperationX struct {
	OperationX
	// TODO (AF): Handling of response (see api1)
	response *Message
}

func (ictx *OperationContext) NewInvokeOperation(urito *URI, area UShort, areaVersion UOctet, service UShort, operation UShort) InvokeOperation {
	// Gets a new TransactionId for operation
	tid := ictx.TransactionId()
	// TODO (AF): Fix length of channel
	ch := make(chan *Message, 10)
	op := &InvokeOperationX{OperationX: OperationX{ictx, tid, ch, urito, area, areaVersion, service, operation, _CREATED}}
	return op
}

func (op *InvokeOperationX) Invoke(body []byte) (*Message, error) {
	if op.status != _CREATED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_INVOKE,
		InteractionStage: MAL_IP_STAGE_INVOKE,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Registers this Invoke Operation in OperationContext
	err := op.ictx.register(op.tid, op)
	if err != nil {
		op.finalize()
		return nil, err
	}
	// Send the INVOKE MAL message
	err = op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the INVOKE_ACK MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_INVOKE_ACK {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	op.status = _ACKNOWLEDGED
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		op.finalize()
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

// Returns the response.
func (op *InvokeOperationX) GetResponse() (*Message, error) {
	if (op.status == _FINAL) && (op.response != nil) {
		if op.response.IsErrorMessage {
			return op.response, errors.New("Error message")
		} else {
			return op.response, nil
		}
	}
	if op.status != _ACKNOWLEDGED {
		return nil, errors.New("Bad operation status")
	}

	// Waits for next MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_INVOKE_RESPONSE {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	op.finalize()
	op.response = msg
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

func (op *InvokeOperationX) onMessage(msg *Message) {
	// Verify the message: service area, version, service, operation
	if op.verify(msg) && (msg.InteractionType == MAL_INTERACTIONTYPE_INVOKE) {
		op.ch <- msg
	} else {
		logger.Errorf("INVOKE Operation (%s,%d) receives Bad message: %+v", *op.urito, op.tid, msg)
	}
}

func (op *InvokeOperationX) onClose() {
	// TODO (AF):
}

// ================================================================================
// Progress Operation

type ProgressOperation interface {
	Operation
	Progress(body []byte) (*Message, error)
	GetUpdate() (*Message, error)
	GetResponse() (*Message, error)
}

type ProgressOperationX struct {
	OperationX
	response *Message
}

func (ictx *OperationContext) NewProgressOperation(urito *URI, area UShort, areaVersion UOctet, service UShort, operation UShort) ProgressOperation {
	// Gets a new TransactionId for operation
	tid := ictx.TransactionId()
	// TODO (AF): Fix length of channel
	ch := make(chan *Message, 10)
	op := &ProgressOperationX{OperationX: OperationX{ictx, tid, ch, urito, area, areaVersion, service, operation, _CREATED}}
	return op
}

func (op *ProgressOperationX) Progress(body []byte) (*Message, error) {
	if op.status != _CREATED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_PROGRESS,
		InteractionStage: MAL_IP_STAGE_PROGRESS,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Registers this Submit Operation in OperationContext
	err := op.ictx.register(op.tid, op)
	if err != nil {
		op.finalize()
		return nil, err
	}
	// Send the SUBMIT MAL message
	err = op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the PROGRESS_ACK MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_PROGRESS_ACK {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	op.status = _ACKNOWLEDGED
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		op.finalize()
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

// Returns next update or nil if there is no more update.
func (op *ProgressOperationX) GetUpdate() (*Message, error) {
	if (op.status != _ACKNOWLEDGED) && (op.status != _PROGRESSING) {
		return nil, errors.New("Bad operation status")
	}

	// Waits for next MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}

	if (msg.InteractionStage != MAL_IP_STAGE_PROGRESS_UPDATE) &&
		(msg.InteractionStage != MAL_IP_STAGE_PROGRESS_RESPONSE) {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}

	if msg.InteractionStage == MAL_IP_STAGE_PROGRESS_UPDATE {
		op.status = _PROGRESSING
		// Verify that the message is ok (ack or error)
		if msg.IsErrorMessage {
			op.finalize()
			return msg, errors.New("Error message")
		} else {
			return msg, nil
		}
	}
	// msg.InteractionStage == MAL_IP_STAGE_PROGRESS_RESPONSE {
	op.response = msg
	op.finalize()
	return nil, nil
}

// Returns the response.
func (op *ProgressOperationX) GetResponse() (*Message, error) {
	if (op.status == _FINAL) && (op.response != nil) {
		if op.response.IsErrorMessage {
			return op.response, errors.New("Error message")
		} else {
			return op.response, nil
		}
	}
	if (op.status != _ACKNOWLEDGED) && (op.status != _PROGRESSING) {
		return nil, errors.New("Bad operation status")
	}

	// Waits for next MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}

	if msg.InteractionStage != MAL_IP_STAGE_PROGRESS_RESPONSE {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	op.finalize()
	op.response = msg
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

func (op *ProgressOperationX) onMessage(msg *Message) {
	// Verify the message: service area, version, service, operation
	if op.verify(msg) && (msg.InteractionType == MAL_INTERACTIONTYPE_PROGRESS) {
		op.ch <- msg
	} else {
		logger.Errorf("PROGRESS Operation (%s,%d) receives Bad message: %+v", *op.urito, op.tid, msg)
	}
}

func (op *ProgressOperationX) onClose() {
	// TODO (AF):
}

// ================================================================================
// Subscriber Operation

type SubscriberOperation interface {
	Operation
	Register(body []byte) (*Message, error)
	GetNotify() (*Message, error)
	Deregister(body []byte) (*Message, error)
}

type SubscriberOperationX struct {
	OperationX
}

func (ictx *OperationContext) NewSubscriberOperation(urito *URI, area UShort, areaVersion UOctet, service UShort, operation UShort) SubscriberOperation {
	// Gets a new TransactionId for operation
	tid := ictx.TransactionId()
	// TODO (AF): Fix length of channel
	ch := make(chan *Message, 10)
	op := &SubscriberOperationX{OperationX: OperationX{ictx, tid, ch, urito, area, areaVersion, service, operation, _CREATED}}
	return op
}

func (op *SubscriberOperationX) Register(body []byte) (*Message, error) {
	// TODO (AF): Be careful we can register anew a Subscriber
	if op.status != _CREATED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _REGISTER_INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_PUBSUB,
		InteractionStage: MAL_IP_STAGE_PUBSUB_REGISTER,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Registers this Operation in OperationContext
	err := op.ictx.register(op.tid, op)
	if err != nil {
		op.finalize()
		return nil, err
	}
	// Send the REGISTER MAL message
	err = op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the REGISTER_ACK MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_PUBSUB_REGISTER_ACK {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		op.finalize()
		return msg, errors.New("Error message")
	} else {
		op.status = _REGISTERED
		return msg, nil
	}
}

// Returns next notify.
func (op *SubscriberOperationX) GetNotify() (*Message, error) {
	if (op.status != _REGISTERED) && (op.status != _REREGISTER_INITIATED) && (op.status != _DEREGISTER_INITIATED) {
		return nil, errors.New("Bad operation status")
	}
	// TODO (AF): Handle _REREGISTER_INITIATED and _DEREGISTER_INITIATED status

	// Waits for next MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_PUBSUB_NOTIFY {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		op.finalize()
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

func (op *SubscriberOperationX) Deregister(body []byte) (*Message, error) {
	if op.status != _REGISTERED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _DEREGISTER_INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_PUBSUB,
		InteractionStage: MAL_IP_STAGE_PUBSUB_DEREGISTER,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Send the DEREGISTER MAL message
	err := op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the DEREGISTER_ACK MAL message, removing useless notify waiting messages
	for {
		msg, more := <-op.ch
		if !more {
			op.finalize()
			logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
			return nil, errors.New("Operation ends")
		}
		if msg.InteractionStage == MAL_IP_STAGE_PUBSUB_NOTIFY {
			continue
		}
		// Verify the message stage
		if msg.InteractionStage != MAL_IP_STAGE_PUBSUB_DEREGISTER_ACK {
			op.finalize()
			logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
			return nil, errors.New("Bad return message")
		}
		op.finalize()
		if msg.IsErrorMessage {
			return msg, errors.New("Error message")
		} else {
			return msg, nil
		}
	}
}

func (op *SubscriberOperationX) onMessage(msg *Message) {
	// Verify the message: service area, version, service, operation
	if op.verify(msg) && (msg.InteractionType == MAL_INTERACTIONTYPE_PUBSUB) {
		op.ch <- msg
	} else {
		logger.Errorf("PUBSUB Operation (%s,%d) receives Bad message: %+v", *op.urito, op.tid, msg)
	}
}

func (op *SubscriberOperationX) onClose() {
	// TODO (AF):
}

// ================================================================================
// Publisher Operation

type PublisherOperation interface {
	Operation
	Register(body []byte) (*Message, error)
	Publish(body []byte) error
	Deregister(body []byte) (*Message, error)
}

type PublisherOperationX struct {
	OperationX
}

func (ictx *OperationContext) NewPublisherOperation(urito *URI, area UShort, areaVersion UOctet, service UShort, operation UShort) PublisherOperation {
	// Gets a new TransactionId for operation
	tid := ictx.TransactionId()
	// TODO (AF): Fix length of channel
	ch := make(chan *Message, 10)
	op := &PublisherOperationX{OperationX: OperationX{ictx, tid, ch, urito, area, areaVersion, service, operation, _CREATED}}
	return op
}

func (op *PublisherOperationX) Register(body []byte) (*Message, error) {
	// TODO (AF): Be careful we can register anew a publisher
	if op.status != _CREATED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _REGISTER_INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_PUBSUB,
		InteractionStage: MAL_IP_STAGE_PUBSUB_PUBLISH_REGISTER,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Registers this Operation in OperationContext
	err := op.ictx.register(op.tid, op)
	if err != nil {
		op.finalize()
		return nil, err
	}
	// Send the PUBLISH_REGISTER MAL message
	err = op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the PUBLISH_REGISTER_ACK MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_PUBSUB_PUBLISH_REGISTER_ACK {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	// Verify that the message is ok (ack or error)
	if msg.IsErrorMessage {
		op.finalize()
		return msg, errors.New("Error message")
	} else {
		op.status = _REGISTERED
		return msg, nil
	}
}

func (op *PublisherOperationX) Publish(body []byte) error {
	if op.status != _REGISTERED {
		return errors.New("Bad operation status")
	}

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_PUBSUB,
		InteractionStage: MAL_IP_STAGE_PUBSUB_PUBLISH,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Send the MAL message
	err := op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return err
	}

	return nil
}

func (op *PublisherOperationX) Deregister(body []byte) (*Message, error) {
	if op.status != _REGISTERED {
		return nil, errors.New("Bad operation status")
	}
	op.status = _DEREGISTER_INITIATED

	msg := &Message{
		UriFrom:          op.ictx.Uri,
		UriTo:            op.urito,
		InteractionType:  MAL_INTERACTIONTYPE_PUBSUB,
		InteractionStage: MAL_IP_STAGE_PUBSUB_PUBLISH_DEREGISTER,
		ServiceArea:      op.area,
		AreaVersion:      op.areaVersion,
		Service:          op.service,
		Operation:        op.operation,
		TransactionId:    op.tid,
		Body:             body,
	}
	// Send the PUBLISH_DEREGISTER MAL message
	err := op.ictx.Ctx.Send(msg)
	if err != nil {
		op.finalize()
		return nil, err
	}

	// Waits for the PUBLISH_DEREGISTER_ACK MAL message
	msg, more := <-op.ch
	if !more {
		op.finalize()
		logger.Debugf("Operation ends: %s, %s", op.ictx.Uri, op.tid)
		return nil, errors.New("Operation ends")
	}
	// Verify the message stage
	if msg.InteractionStage != MAL_IP_STAGE_PUBSUB_PUBLISH_DEREGISTER_ACK {
		op.finalize()
		logger.Errorf("Bad return message, operation (%s, %s), stage %d", op.ictx.Uri, op.tid, msg.InteractionStage)
		return nil, errors.New("Bad return message")
	}
	op.finalize()
	if msg.IsErrorMessage {
		return msg, errors.New("Error message")
	} else {
		return msg, nil
	}
}

func (op *PublisherOperationX) onMessage(msg *Message) {
	// Verify the message: service area, version, service, operation
	if op.verify(msg) && (msg.InteractionType == MAL_INTERACTIONTYPE_PUBSUB) {
		op.ch <- msg
	} else {
		logger.Errorf("PUBSUB Operation (%s,%d) receives Bad message: %+v", *op.urito, op.tid, msg)
	}
}

func (op *PublisherOperationX) onClose() {
	// TODO (AF):
}

// ================================================================================
// Defines Listener interface used by context to route MAL messages

func (ictx *OperationContext) OnMessage(msg *Message) error {
	// Note (AF): The generated TransactionId is unique for this requesting URI so we
	// can use it as key to retrieve the Operation (This is more restrictive than the
	// MAL API (see section 3.2).
	to, ok := ictx.handlers[msg.TransactionId]
	if ok {
		logger.Debugf("onMessage %t", to)
		to.onMessage(msg)
		logger.Debugf("OnMessageMessage transmitted: %s", msg)
	} else {
		logger.Debugf("Cannot route message to: %s?TransactionId=", msg.UriTo, msg.TransactionId)
	}
	return nil
}

func (ictx *OperationContext) OnClose() error {
	logger.Infof("close EndPoint: %s", ictx.Uri)
	for tid, handler := range ictx.handlers {
		logger.Debugf("close operation: %d", tid)
		handler.onClose()
	}
	return nil
}
