/**
 * MIT License
 *
 * Copyright (c) 2018 - 2020 CNES
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
package com

import (
  "github.com/CNES/ccsdsmo-malgo/mal"
)

// Defines ObjectType type

type ObjectType struct {
  Area mal.UShort
  Service mal.UShort
  Version mal.UOctet
  Number mal.UShort
}

var (
  NullObjectType *ObjectType = nil
)
func NewObjectType() *ObjectType {
  return new(ObjectType)
}

// ================================================================================
// Defines ObjectType type as a MAL Composite

func (receiver *ObjectType) Composite() mal.Composite {
  return receiver
}

// ================================================================================
// Defines ObjectType type as a MAL Element

const OBJECTTYPE_TYPE_SHORT_FORM mal.Integer = 1
const OBJECTTYPE_SHORT_FORM mal.Long = 0x2000001000001

// Registers ObjectType type for polymorphism handling
func init() {
  mal.RegisterMALElement(OBJECTTYPE_SHORT_FORM, NullObjectType)
}

// Returns the absolute short form of the element type.
func (receiver *ObjectType) GetShortForm() mal.Long {
  return OBJECTTYPE_SHORT_FORM
}

// Returns the number of the area this element type belongs to.
func (receiver *ObjectType) GetAreaNumber() mal.UShort {
  return AREA_NUMBER
}

// Returns the version of the area this element type belongs to.
func (receiver *ObjectType) GetAreaVersion() mal.UOctet {
  return AREA_VERSION
}

// Returns the number of the service this element type belongs to.
func (receiver *ObjectType) GetServiceNumber() mal.UShort {
    return mal.NULL_SERVICE_NUMBER
}

// Returns the relative short form of the element type.
func (receiver *ObjectType) GetTypeShortForm() mal.Integer {
  return OBJECTTYPE_TYPE_SHORT_FORM
}

// Allows the creation of an element in a generic way, i.e., using the MAL Element polymorphism.
func (receiver *ObjectType) CreateElement() mal.Element {
  return new(ObjectType)
}

func (receiver *ObjectType) IsNull() bool {
  return receiver == nil
}

func (receiver *ObjectType) Null() mal.Element {
  return NullObjectType
}

// Encodes this element using the supplied encoder.
// @param encoder The encoder to use, must not be null.
func (receiver *ObjectType) Encode(encoder mal.Encoder) error {
  specific := encoder.LookupSpecific(OBJECTTYPE_SHORT_FORM)
  if specific != nil {
    return specific(receiver, encoder)
  }

  err := encoder.EncodeUShort(&receiver.Area)
  if err != nil {
    return err
  }
  err = encoder.EncodeUShort(&receiver.Service)
  if err != nil {
    return err
  }
  err = encoder.EncodeUOctet(&receiver.Version)
  if err != nil {
    return err
  }
  err = encoder.EncodeUShort(&receiver.Number)
  if err != nil {
    return err
  }

  return nil
}

// Decodes an instance of this element type using the supplied decoder.
// @param decoder The decoder to use, must not be null.
// @return the decoded instance, may be not the same instance as this Element.
func (receiver *ObjectType) Decode(decoder mal.Decoder) (mal.Element, error) {
  specific := decoder.LookupSpecific(OBJECTTYPE_SHORT_FORM)
  if specific != nil {
    return specific(decoder)
  }

  Area, err := decoder.DecodeUShort()
  if err != nil {
    return nil, err
  }
  Service, err := decoder.DecodeUShort()
  if err != nil {
    return nil, err
  }
  Version, err := decoder.DecodeUOctet()
  if err != nil {
    return nil, err
  }
  Number, err := decoder.DecodeUShort()
  if err != nil {
    return nil, err
  }

  var composite = ObjectType {
    Area: *Area,
    Service: *Service,
    Version: *Version,
    Number: *Number,
  }
  return &composite, nil
}
