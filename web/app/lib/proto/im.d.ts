import * as $protobuf from "protobufjs";
import Long = require("long");

/** Namespace api. */
export namespace api {

    /** Namespace im. */
    namespace im {

        /** Namespace v1. */
        namespace v1 {

            /** Represents an AuthService */
            class AuthService extends $protobuf.rpc.Service {

                /**
                 * Constructs a new AuthService service.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 */
                constructor(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean);

                /**
                 * Creates new AuthService service using the specified rpc implementation.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 * @returns RPC service. Useful where requests and/or responses are streamed.
                 */
                static create(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean): AuthService;

                /** Calls Register. */
                register: api.im.v1.AuthService.Register;

                /** Calls Login. */
                login: api.im.v1.AuthService.Login;
            }

            namespace AuthService {

                /**
                 * Callback as used by {@link api.im.v1.AuthService#register}.
                 * @param error Error, if any
                 * @param [response] RegisterReply
                 */
                type RegisterCallback = (error: (Error|null), response?: api.im.v1.RegisterReply) => void;

                /** Calls Register. */
                type Register = {
                  (request: api.im.v1.IRegisterRequest, callback: api.im.v1.AuthService.RegisterCallback): void;
                  (request: api.im.v1.IRegisterRequest): Promise<api.im.v1.RegisterReply>;
                  readonly name: "Register";
                  readonly path: "/api.im.v1.AuthService/Register";
                  readonly requestType: "RegisterRequest";
                  readonly responseType: "RegisterReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };

                /**
                 * Callback as used by {@link api.im.v1.AuthService#login}.
                 * @param error Error, if any
                 * @param [response] LoginReply
                 */
                type LoginCallback = (error: (Error|null), response?: api.im.v1.LoginReply) => void;

                /** Calls Login. */
                type Login = {
                  (request: api.im.v1.ILoginRequest, callback: api.im.v1.AuthService.LoginCallback): void;
                  (request: api.im.v1.ILoginRequest): Promise<api.im.v1.LoginReply>;
                  readonly name: "Login";
                  readonly path: "/api.im.v1.AuthService/Login";
                  readonly requestType: "LoginRequest";
                  readonly responseType: "LoginReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };
            }

            /**
             * Properties of a RegisterRequest.
             * @deprecated Use api.im.v1.RegisterRequest.$Properties instead.
             */
            interface IRegisterRequest extends api.im.v1.RegisterRequest.$Properties {
            }

            /** Represents a RegisterRequest. */
            class RegisterRequest {

                /**
                 * Constructs a new RegisterRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.RegisterRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** RegisterRequest username. */
                username: string;

                /** RegisterRequest password. */
                password: string;

                /**
                 * Creates a new RegisterRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns RegisterRequest instance
                 */
                static create(properties: api.im.v1.RegisterRequest.$Shape): api.im.v1.RegisterRequest & api.im.v1.RegisterRequest.$Shape;
                static create(properties?: api.im.v1.RegisterRequest.$Properties): api.im.v1.RegisterRequest;

                /**
                 * Encodes the specified RegisterRequest message. Does not implicitly {@link api.im.v1.RegisterRequest.verify|verify} messages.
                 * @param message RegisterRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.RegisterRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified RegisterRequest message, length delimited. Does not implicitly {@link api.im.v1.RegisterRequest.verify|verify} messages.
                 * @param message RegisterRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.RegisterRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a RegisterRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.RegisterRequest & api.im.v1.RegisterRequest.$Shape} RegisterRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.RegisterRequest & api.im.v1.RegisterRequest.$Shape;

                /**
                 * Decodes a RegisterRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.RegisterRequest & api.im.v1.RegisterRequest.$Shape} RegisterRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.RegisterRequest & api.im.v1.RegisterRequest.$Shape;

                /**
                 * Verifies a RegisterRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a RegisterRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns RegisterRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.RegisterRequest;

                /**
                 * Creates a plain object from a RegisterRequest message. Also converts values to other types if specified.
                 * @param message RegisterRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.RegisterRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this RegisterRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for RegisterRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace RegisterRequest {

                /** Properties of a RegisterRequest. */
                interface $Properties {

                    /** RegisterRequest username */
                    username?: (string|null);

                    /** RegisterRequest password */
                    password?: (string|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a RegisterRequest. */
                type $Shape = api.im.v1.RegisterRequest.$Properties;
            }

            /**
             * Properties of a RegisterReply.
             * @deprecated Use api.im.v1.RegisterReply.$Properties instead.
             */
            interface IRegisterReply extends api.im.v1.RegisterReply.$Properties {
            }

            /** Represents a RegisterReply. */
            class RegisterReply {

                /**
                 * Constructs a new RegisterReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.RegisterReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** RegisterReply userId. */
                userId: (number|Long);

                /**
                 * Creates a new RegisterReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns RegisterReply instance
                 */
                static create(properties: api.im.v1.RegisterReply.$Shape): api.im.v1.RegisterReply & api.im.v1.RegisterReply.$Shape;
                static create(properties?: api.im.v1.RegisterReply.$Properties): api.im.v1.RegisterReply;

                /**
                 * Encodes the specified RegisterReply message. Does not implicitly {@link api.im.v1.RegisterReply.verify|verify} messages.
                 * @param message RegisterReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.RegisterReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified RegisterReply message, length delimited. Does not implicitly {@link api.im.v1.RegisterReply.verify|verify} messages.
                 * @param message RegisterReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.RegisterReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a RegisterReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.RegisterReply & api.im.v1.RegisterReply.$Shape} RegisterReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.RegisterReply & api.im.v1.RegisterReply.$Shape;

                /**
                 * Decodes a RegisterReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.RegisterReply & api.im.v1.RegisterReply.$Shape} RegisterReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.RegisterReply & api.im.v1.RegisterReply.$Shape;

                /**
                 * Verifies a RegisterReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a RegisterReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns RegisterReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.RegisterReply;

                /**
                 * Creates a plain object from a RegisterReply message. Also converts values to other types if specified.
                 * @param message RegisterReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.RegisterReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this RegisterReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for RegisterReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace RegisterReply {

                /** Properties of a RegisterReply. */
                interface $Properties {

                    /** RegisterReply userId */
                    userId?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a RegisterReply. */
                type $Shape = api.im.v1.RegisterReply.$Properties;
            }

            /**
             * Properties of a LoginRequest.
             * @deprecated Use api.im.v1.LoginRequest.$Properties instead.
             */
            interface ILoginRequest extends api.im.v1.LoginRequest.$Properties {
            }

            /** Represents a LoginRequest. */
            class LoginRequest {

                /**
                 * Constructs a new LoginRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.LoginRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** LoginRequest username. */
                username: string;

                /** LoginRequest password. */
                password: string;

                /** LoginRequest deviceId. */
                deviceId: string;

                /**
                 * Creates a new LoginRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns LoginRequest instance
                 */
                static create(properties: api.im.v1.LoginRequest.$Shape): api.im.v1.LoginRequest & api.im.v1.LoginRequest.$Shape;
                static create(properties?: api.im.v1.LoginRequest.$Properties): api.im.v1.LoginRequest;

                /**
                 * Encodes the specified LoginRequest message. Does not implicitly {@link api.im.v1.LoginRequest.verify|verify} messages.
                 * @param message LoginRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.LoginRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified LoginRequest message, length delimited. Does not implicitly {@link api.im.v1.LoginRequest.verify|verify} messages.
                 * @param message LoginRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.LoginRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a LoginRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.LoginRequest & api.im.v1.LoginRequest.$Shape} LoginRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.LoginRequest & api.im.v1.LoginRequest.$Shape;

                /**
                 * Decodes a LoginRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.LoginRequest & api.im.v1.LoginRequest.$Shape} LoginRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.LoginRequest & api.im.v1.LoginRequest.$Shape;

                /**
                 * Verifies a LoginRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a LoginRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns LoginRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.LoginRequest;

                /**
                 * Creates a plain object from a LoginRequest message. Also converts values to other types if specified.
                 * @param message LoginRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.LoginRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this LoginRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for LoginRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace LoginRequest {

                /** Properties of a LoginRequest. */
                interface $Properties {

                    /** LoginRequest username */
                    username?: (string|null);

                    /** LoginRequest password */
                    password?: (string|null);

                    /** LoginRequest deviceId */
                    deviceId?: (string|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a LoginRequest. */
                type $Shape = api.im.v1.LoginRequest.$Properties;
            }

            /**
             * Properties of a LoginReply.
             * @deprecated Use api.im.v1.LoginReply.$Properties instead.
             */
            interface ILoginReply extends api.im.v1.LoginReply.$Properties {
            }

            /** Represents a LoginReply. */
            class LoginReply {

                /**
                 * Constructs a new LoginReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.LoginReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** LoginReply userId. */
                userId: (number|Long);

                /** LoginReply token. */
                token: string;

                /**
                 * Creates a new LoginReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns LoginReply instance
                 */
                static create(properties: api.im.v1.LoginReply.$Shape): api.im.v1.LoginReply & api.im.v1.LoginReply.$Shape;
                static create(properties?: api.im.v1.LoginReply.$Properties): api.im.v1.LoginReply;

                /**
                 * Encodes the specified LoginReply message. Does not implicitly {@link api.im.v1.LoginReply.verify|verify} messages.
                 * @param message LoginReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.LoginReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified LoginReply message, length delimited. Does not implicitly {@link api.im.v1.LoginReply.verify|verify} messages.
                 * @param message LoginReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.LoginReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a LoginReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.LoginReply & api.im.v1.LoginReply.$Shape} LoginReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.LoginReply & api.im.v1.LoginReply.$Shape;

                /**
                 * Decodes a LoginReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.LoginReply & api.im.v1.LoginReply.$Shape} LoginReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.LoginReply & api.im.v1.LoginReply.$Shape;

                /**
                 * Verifies a LoginReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a LoginReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns LoginReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.LoginReply;

                /**
                 * Creates a plain object from a LoginReply message. Also converts values to other types if specified.
                 * @param message LoginReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.LoginReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this LoginReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for LoginReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace LoginReply {

                /** Properties of a LoginReply. */
                interface $Properties {

                    /** LoginReply userId */
                    userId?: (number|Long|null);

                    /** LoginReply token */
                    token?: (string|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a LoginReply. */
                type $Shape = api.im.v1.LoginReply.$Properties;
            }

            /** Represents a DispatchService */
            class DispatchService extends $protobuf.rpc.Service {

                /**
                 * Constructs a new DispatchService service.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 */
                constructor(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean);

                /**
                 * Creates new DispatchService service using the specified rpc implementation.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 * @returns RPC service. Useful where requests and/or responses are streamed.
                 */
                static create(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean): DispatchService;

                /** Calls Gateway. */
                gateway: api.im.v1.DispatchService.Gateway;
            }

            namespace DispatchService {

                /**
                 * Callback as used by {@link api.im.v1.DispatchService#gateway}.
                 * @param error Error, if any
                 * @param [response] GetGatewayReply
                 */
                type GatewayCallback = (error: (Error|null), response?: api.im.v1.GetGatewayReply) => void;

                /** Calls Gateway. */
                type Gateway = {
                  (request: api.im.v1.IGetGatewayRequest, callback: api.im.v1.DispatchService.GatewayCallback): void;
                  (request: api.im.v1.IGetGatewayRequest): Promise<api.im.v1.GetGatewayReply>;
                  readonly name: "Gateway";
                  readonly path: "/api.im.v1.DispatchService/Gateway";
                  readonly requestType: "GetGatewayRequest";
                  readonly responseType: "GetGatewayReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };
            }

            /**
             * Properties of a GetGatewayRequest.
             * @deprecated Use api.im.v1.GetGatewayRequest.$Properties instead.
             */
            interface IGetGatewayRequest extends api.im.v1.GetGatewayRequest.$Properties {
            }

            /** Represents a GetGatewayRequest. */
            class GetGatewayRequest {

                /**
                 * Constructs a new GetGatewayRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.GetGatewayRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** GetGatewayRequest userId. */
                userId: (number|Long);

                /**
                 * Creates a new GetGatewayRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns GetGatewayRequest instance
                 */
                static create(properties: api.im.v1.GetGatewayRequest.$Shape): api.im.v1.GetGatewayRequest & api.im.v1.GetGatewayRequest.$Shape;
                static create(properties?: api.im.v1.GetGatewayRequest.$Properties): api.im.v1.GetGatewayRequest;

                /**
                 * Encodes the specified GetGatewayRequest message. Does not implicitly {@link api.im.v1.GetGatewayRequest.verify|verify} messages.
                 * @param message GetGatewayRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.GetGatewayRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified GetGatewayRequest message, length delimited. Does not implicitly {@link api.im.v1.GetGatewayRequest.verify|verify} messages.
                 * @param message GetGatewayRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.GetGatewayRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a GetGatewayRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.GetGatewayRequest & api.im.v1.GetGatewayRequest.$Shape} GetGatewayRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.GetGatewayRequest & api.im.v1.GetGatewayRequest.$Shape;

                /**
                 * Decodes a GetGatewayRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.GetGatewayRequest & api.im.v1.GetGatewayRequest.$Shape} GetGatewayRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.GetGatewayRequest & api.im.v1.GetGatewayRequest.$Shape;

                /**
                 * Verifies a GetGatewayRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a GetGatewayRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns GetGatewayRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.GetGatewayRequest;

                /**
                 * Creates a plain object from a GetGatewayRequest message. Also converts values to other types if specified.
                 * @param message GetGatewayRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.GetGatewayRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this GetGatewayRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for GetGatewayRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace GetGatewayRequest {

                /** Properties of a GetGatewayRequest. */
                interface $Properties {

                    /** GetGatewayRequest userId */
                    userId?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a GetGatewayRequest. */
                type $Shape = api.im.v1.GetGatewayRequest.$Properties;
            }

            /**
             * Properties of a GetGatewayReply.
             * @deprecated Use api.im.v1.GetGatewayReply.$Properties instead.
             */
            interface IGetGatewayReply extends api.im.v1.GetGatewayReply.$Properties {
            }

            /** Represents a GetGatewayReply. */
            class GetGatewayReply {

                /**
                 * Constructs a new GetGatewayReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.GetGatewayReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** GetGatewayReply gatewayUrl. */
                gatewayUrl: string;

                /** GetGatewayReply gatewayUrls. */
                gatewayUrls: string[];

                /**
                 * Creates a new GetGatewayReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns GetGatewayReply instance
                 */
                static create(properties: api.im.v1.GetGatewayReply.$Shape): api.im.v1.GetGatewayReply & api.im.v1.GetGatewayReply.$Shape;
                static create(properties?: api.im.v1.GetGatewayReply.$Properties): api.im.v1.GetGatewayReply;

                /**
                 * Encodes the specified GetGatewayReply message. Does not implicitly {@link api.im.v1.GetGatewayReply.verify|verify} messages.
                 * @param message GetGatewayReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.GetGatewayReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified GetGatewayReply message, length delimited. Does not implicitly {@link api.im.v1.GetGatewayReply.verify|verify} messages.
                 * @param message GetGatewayReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.GetGatewayReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a GetGatewayReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.GetGatewayReply & api.im.v1.GetGatewayReply.$Shape} GetGatewayReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.GetGatewayReply & api.im.v1.GetGatewayReply.$Shape;

                /**
                 * Decodes a GetGatewayReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.GetGatewayReply & api.im.v1.GetGatewayReply.$Shape} GetGatewayReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.GetGatewayReply & api.im.v1.GetGatewayReply.$Shape;

                /**
                 * Verifies a GetGatewayReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a GetGatewayReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns GetGatewayReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.GetGatewayReply;

                /**
                 * Creates a plain object from a GetGatewayReply message. Also converts values to other types if specified.
                 * @param message GetGatewayReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.GetGatewayReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this GetGatewayReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for GetGatewayReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace GetGatewayReply {

                /** Properties of a GetGatewayReply. */
                interface $Properties {

                    /** GetGatewayReply gatewayUrl */
                    gatewayUrl?: (string|null);

                    /** GetGatewayReply gatewayUrls */
                    gatewayUrls?: (string[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a GetGatewayReply. */
                type $Shape = api.im.v1.GetGatewayReply.$Properties;
            }

            /** Represents a MessageService */
            class MessageService extends $protobuf.rpc.Service {

                /**
                 * Constructs a new MessageService service.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 */
                constructor(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean);

                /**
                 * Creates new MessageService service using the specified rpc implementation.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 * @returns RPC service. Useful where requests and/or responses are streamed.
                 */
                static create(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean): MessageService;

                /** Calls SendMessage. */
                sendMessage: api.im.v1.MessageService.SendMessage;

                /** Calls PullMessages. */
                pullMessages: api.im.v1.MessageService.PullMessages;
            }

            namespace MessageService {

                /**
                 * Callback as used by {@link api.im.v1.MessageService#sendMessage}.
                 * @param error Error, if any
                 * @param [response] SendMessageReply
                 */
                type SendMessageCallback = (error: (Error|null), response?: api.im.v1.SendMessageReply) => void;

                /** Calls SendMessage. */
                type SendMessage = {
                  (request: api.im.v1.ISendMessageRequest, callback: api.im.v1.MessageService.SendMessageCallback): void;
                  (request: api.im.v1.ISendMessageRequest): Promise<api.im.v1.SendMessageReply>;
                  readonly name: "SendMessage";
                  readonly path: "/api.im.v1.MessageService/SendMessage";
                  readonly requestType: "SendMessageRequest";
                  readonly responseType: "SendMessageReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };

                /**
                 * Callback as used by {@link api.im.v1.MessageService#pullMessages}.
                 * @param error Error, if any
                 * @param [response] PullReply
                 */
                type PullMessagesCallback = (error: (Error|null), response?: api.im.v1.PullReply) => void;

                /** Calls PullMessages. */
                type PullMessages = {
                  (request: api.im.v1.IPullRequest, callback: api.im.v1.MessageService.PullMessagesCallback): void;
                  (request: api.im.v1.IPullRequest): Promise<api.im.v1.PullReply>;
                  readonly name: "PullMessages";
                  readonly path: "/api.im.v1.MessageService/PullMessages";
                  readonly requestType: "PullRequest";
                  readonly responseType: "PullReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };
            }

            /** MsgType enum. */
            enum MsgType {

                /** MSG_TYPE_UNSPECIFIED value */
                MSG_TYPE_UNSPECIFIED = 0,

                /** MSG_TYPE_TEXT value */
                MSG_TYPE_TEXT = 1,

                /** MSG_TYPE_IMAGE value */
                MSG_TYPE_IMAGE = 2,

                /** MSG_TYPE_FILE value */
                MSG_TYPE_FILE = 3,

                /** MSG_TYPE_VOICE value */
                MSG_TYPE_VOICE = 4
            }

            /**
             * Properties of a SendMessageRequest.
             * @deprecated Use api.im.v1.SendMessageRequest.$Properties instead.
             */
            interface ISendMessageRequest extends api.im.v1.SendMessageRequest.$Properties {
            }

            /** Represents a SendMessageRequest. */
            class SendMessageRequest {

                /**
                 * Constructs a new SendMessageRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.SendMessageRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** SendMessageRequest topic. */
                topic: string;

                /** SendMessageRequest msgType. */
                msgType: api.im.v1.MsgType;

                /** SendMessageRequest content. */
                content: Uint8Array;

                /** SendMessageRequest clientMsgId. */
                clientMsgId: string;

                /** SendMessageRequest mentionedUserIds. */
                mentionedUserIds: (number|Long)[];

                /**
                 * Creates a new SendMessageRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns SendMessageRequest instance
                 */
                static create(properties: api.im.v1.SendMessageRequest.$Shape): api.im.v1.SendMessageRequest & api.im.v1.SendMessageRequest.$Shape;
                static create(properties?: api.im.v1.SendMessageRequest.$Properties): api.im.v1.SendMessageRequest;

                /**
                 * Encodes the specified SendMessageRequest message. Does not implicitly {@link api.im.v1.SendMessageRequest.verify|verify} messages.
                 * @param message SendMessageRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.SendMessageRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified SendMessageRequest message, length delimited. Does not implicitly {@link api.im.v1.SendMessageRequest.verify|verify} messages.
                 * @param message SendMessageRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.SendMessageRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a SendMessageRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.SendMessageRequest & api.im.v1.SendMessageRequest.$Shape} SendMessageRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.SendMessageRequest & api.im.v1.SendMessageRequest.$Shape;

                /**
                 * Decodes a SendMessageRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.SendMessageRequest & api.im.v1.SendMessageRequest.$Shape} SendMessageRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.SendMessageRequest & api.im.v1.SendMessageRequest.$Shape;

                /**
                 * Verifies a SendMessageRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a SendMessageRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns SendMessageRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.SendMessageRequest;

                /**
                 * Creates a plain object from a SendMessageRequest message. Also converts values to other types if specified.
                 * @param message SendMessageRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.SendMessageRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this SendMessageRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for SendMessageRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace SendMessageRequest {

                /** Properties of a SendMessageRequest. */
                interface $Properties {

                    /** SendMessageRequest topic */
                    topic?: (string|null);

                    /** SendMessageRequest msgType */
                    msgType?: (api.im.v1.MsgType|null);

                    /** SendMessageRequest content */
                    content?: (Uint8Array|null);

                    /** SendMessageRequest clientMsgId */
                    clientMsgId?: (string|null);

                    /** SendMessageRequest mentionedUserIds */
                    mentionedUserIds?: ((number|Long)[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a SendMessageRequest. */
                type $Shape = api.im.v1.SendMessageRequest.$Properties;
            }

            /**
             * Properties of a SendMessageReply.
             * @deprecated Use api.im.v1.SendMessageReply.$Properties instead.
             */
            interface ISendMessageReply extends api.im.v1.SendMessageReply.$Properties {
            }

            /** Represents a SendMessageReply. */
            class SendMessageReply {

                /**
                 * Constructs a new SendMessageReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.SendMessageReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** SendMessageReply clientMsgId. */
                clientMsgId: string;

                /** SendMessageReply msgId. */
                msgId: (number|Long);

                /** SendMessageReply timestamp. */
                timestamp: (number|Long);

                /** SendMessageReply topicSeq. */
                topicSeq: (number|Long);

                /**
                 * Creates a new SendMessageReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns SendMessageReply instance
                 */
                static create(properties: api.im.v1.SendMessageReply.$Shape): api.im.v1.SendMessageReply & api.im.v1.SendMessageReply.$Shape;
                static create(properties?: api.im.v1.SendMessageReply.$Properties): api.im.v1.SendMessageReply;

                /**
                 * Encodes the specified SendMessageReply message. Does not implicitly {@link api.im.v1.SendMessageReply.verify|verify} messages.
                 * @param message SendMessageReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.SendMessageReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified SendMessageReply message, length delimited. Does not implicitly {@link api.im.v1.SendMessageReply.verify|verify} messages.
                 * @param message SendMessageReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.SendMessageReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a SendMessageReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.SendMessageReply & api.im.v1.SendMessageReply.$Shape} SendMessageReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.SendMessageReply & api.im.v1.SendMessageReply.$Shape;

                /**
                 * Decodes a SendMessageReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.SendMessageReply & api.im.v1.SendMessageReply.$Shape} SendMessageReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.SendMessageReply & api.im.v1.SendMessageReply.$Shape;

                /**
                 * Verifies a SendMessageReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a SendMessageReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns SendMessageReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.SendMessageReply;

                /**
                 * Creates a plain object from a SendMessageReply message. Also converts values to other types if specified.
                 * @param message SendMessageReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.SendMessageReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this SendMessageReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for SendMessageReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace SendMessageReply {

                /** Properties of a SendMessageReply. */
                interface $Properties {

                    /** SendMessageReply clientMsgId */
                    clientMsgId?: (string|null);

                    /** SendMessageReply msgId */
                    msgId?: (number|Long|null);

                    /** SendMessageReply timestamp */
                    timestamp?: (number|Long|null);

                    /** SendMessageReply topicSeq */
                    topicSeq?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a SendMessageReply. */
                type $Shape = api.im.v1.SendMessageReply.$Properties;
            }

            /**
             * Properties of a MessagePush.
             * @deprecated Use api.im.v1.MessagePush.$Properties instead.
             */
            interface IMessagePush extends api.im.v1.MessagePush.$Properties {
            }

            /** Represents a MessagePush. */
            class MessagePush {

                /**
                 * Constructs a new MessagePush.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.MessagePush.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** MessagePush msgId. */
                msgId: (number|Long);

                /** MessagePush topic. */
                topic: string;

                /** MessagePush senderId. */
                senderId: (number|Long);

                /** MessagePush msgType. */
                msgType: number;

                /** MessagePush content. */
                content: Uint8Array;

                /** MessagePush timestamp. */
                timestamp: (number|Long);

                /** MessagePush topicSeq. */
                topicSeq: (number|Long);

                /** MessagePush clientMsgId. */
                clientMsgId: string;

                /**
                 * Creates a new MessagePush instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns MessagePush instance
                 */
                static create(properties: api.im.v1.MessagePush.$Shape): api.im.v1.MessagePush & api.im.v1.MessagePush.$Shape;
                static create(properties?: api.im.v1.MessagePush.$Properties): api.im.v1.MessagePush;

                /**
                 * Encodes the specified MessagePush message. Does not implicitly {@link api.im.v1.MessagePush.verify|verify} messages.
                 * @param message MessagePush message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.MessagePush.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified MessagePush message, length delimited. Does not implicitly {@link api.im.v1.MessagePush.verify|verify} messages.
                 * @param message MessagePush message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.MessagePush.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a MessagePush message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.MessagePush & api.im.v1.MessagePush.$Shape} MessagePush
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.MessagePush & api.im.v1.MessagePush.$Shape;

                /**
                 * Decodes a MessagePush message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.MessagePush & api.im.v1.MessagePush.$Shape} MessagePush
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.MessagePush & api.im.v1.MessagePush.$Shape;

                /**
                 * Verifies a MessagePush message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a MessagePush message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns MessagePush
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.MessagePush;

                /**
                 * Creates a plain object from a MessagePush message. Also converts values to other types if specified.
                 * @param message MessagePush
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.MessagePush, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this MessagePush to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for MessagePush
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace MessagePush {

                /** Properties of a MessagePush. */
                interface $Properties {

                    /** MessagePush msgId */
                    msgId?: (number|Long|null);

                    /** MessagePush topic */
                    topic?: (string|null);

                    /** MessagePush senderId */
                    senderId?: (number|Long|null);

                    /** MessagePush msgType */
                    msgType?: (number|null);

                    /** MessagePush content */
                    content?: (Uint8Array|null);

                    /** MessagePush timestamp */
                    timestamp?: (number|Long|null);

                    /** MessagePush topicSeq */
                    topicSeq?: (number|Long|null);

                    /** MessagePush clientMsgId */
                    clientMsgId?: (string|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a MessagePush. */
                type $Shape = api.im.v1.MessagePush.$Properties;
            }

            /**
             * Properties of an UpstreamMessage.
             * @deprecated Use api.im.v1.UpstreamMessage.$Properties instead.
             */
            interface IUpstreamMessage extends api.im.v1.UpstreamMessage.$Properties {
            }

            /** Represents an UpstreamMessage. */
            class UpstreamMessage {

                /**
                 * Constructs a new UpstreamMessage.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.UpstreamMessage.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** UpstreamMessage senderId. */
                senderId: (number|Long);

                /** UpstreamMessage topic. */
                topic: string;

                /** UpstreamMessage msgType. */
                msgType: number;

                /** UpstreamMessage content. */
                content: Uint8Array;

                /** UpstreamMessage clientMsgId. */
                clientMsgId: string;

                /** UpstreamMessage timestamp. */
                timestamp: (number|Long);

                /** UpstreamMessage mentionedUserIds. */
                mentionedUserIds: (number|Long)[];

                /**
                 * Creates a new UpstreamMessage instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns UpstreamMessage instance
                 */
                static create(properties: api.im.v1.UpstreamMessage.$Shape): api.im.v1.UpstreamMessage & api.im.v1.UpstreamMessage.$Shape;
                static create(properties?: api.im.v1.UpstreamMessage.$Properties): api.im.v1.UpstreamMessage;

                /**
                 * Encodes the specified UpstreamMessage message. Does not implicitly {@link api.im.v1.UpstreamMessage.verify|verify} messages.
                 * @param message UpstreamMessage message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.UpstreamMessage.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified UpstreamMessage message, length delimited. Does not implicitly {@link api.im.v1.UpstreamMessage.verify|verify} messages.
                 * @param message UpstreamMessage message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.UpstreamMessage.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an UpstreamMessage message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.UpstreamMessage & api.im.v1.UpstreamMessage.$Shape} UpstreamMessage
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.UpstreamMessage & api.im.v1.UpstreamMessage.$Shape;

                /**
                 * Decodes an UpstreamMessage message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.UpstreamMessage & api.im.v1.UpstreamMessage.$Shape} UpstreamMessage
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.UpstreamMessage & api.im.v1.UpstreamMessage.$Shape;

                /**
                 * Verifies an UpstreamMessage message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an UpstreamMessage message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns UpstreamMessage
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.UpstreamMessage;

                /**
                 * Creates a plain object from an UpstreamMessage message. Also converts values to other types if specified.
                 * @param message UpstreamMessage
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.UpstreamMessage, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this UpstreamMessage to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for UpstreamMessage
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace UpstreamMessage {

                /** Properties of an UpstreamMessage. */
                interface $Properties {

                    /** UpstreamMessage senderId */
                    senderId?: (number|Long|null);

                    /** UpstreamMessage topic */
                    topic?: (string|null);

                    /** UpstreamMessage msgType */
                    msgType?: (number|null);

                    /** UpstreamMessage content */
                    content?: (Uint8Array|null);

                    /** UpstreamMessage clientMsgId */
                    clientMsgId?: (string|null);

                    /** UpstreamMessage timestamp */
                    timestamp?: (number|Long|null);

                    /** UpstreamMessage mentionedUserIds */
                    mentionedUserIds?: ((number|Long)[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an UpstreamMessage. */
                type $Shape = api.im.v1.UpstreamMessage.$Properties;
            }

            /**
             * Properties of a PullRequest.
             * @deprecated Use api.im.v1.PullRequest.$Properties instead.
             */
            interface IPullRequest extends api.im.v1.PullRequest.$Properties {
            }

            /** Represents a PullRequest. */
            class PullRequest {

                /**
                 * Constructs a new PullRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.PullRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** PullRequest topic. */
                topic: string;

                /** PullRequest lastSeq. */
                lastSeq: (number|Long);

                /** PullRequest limit. */
                limit: number;

                /**
                 * Creates a new PullRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns PullRequest instance
                 */
                static create(properties: api.im.v1.PullRequest.$Shape): api.im.v1.PullRequest & api.im.v1.PullRequest.$Shape;
                static create(properties?: api.im.v1.PullRequest.$Properties): api.im.v1.PullRequest;

                /**
                 * Encodes the specified PullRequest message. Does not implicitly {@link api.im.v1.PullRequest.verify|verify} messages.
                 * @param message PullRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.PullRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified PullRequest message, length delimited. Does not implicitly {@link api.im.v1.PullRequest.verify|verify} messages.
                 * @param message PullRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.PullRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a PullRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.PullRequest & api.im.v1.PullRequest.$Shape} PullRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.PullRequest & api.im.v1.PullRequest.$Shape;

                /**
                 * Decodes a PullRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.PullRequest & api.im.v1.PullRequest.$Shape} PullRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.PullRequest & api.im.v1.PullRequest.$Shape;

                /**
                 * Verifies a PullRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a PullRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns PullRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.PullRequest;

                /**
                 * Creates a plain object from a PullRequest message. Also converts values to other types if specified.
                 * @param message PullRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.PullRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this PullRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for PullRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace PullRequest {

                /** Properties of a PullRequest. */
                interface $Properties {

                    /** PullRequest topic */
                    topic?: (string|null);

                    /** PullRequest lastSeq */
                    lastSeq?: (number|Long|null);

                    /** PullRequest limit */
                    limit?: (number|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a PullRequest. */
                type $Shape = api.im.v1.PullRequest.$Properties;
            }

            /**
             * Properties of a PullMessage.
             * @deprecated Use api.im.v1.PullMessage.$Properties instead.
             */
            interface IPullMessage extends api.im.v1.PullMessage.$Properties {
            }

            /** Represents a PullMessage. */
            class PullMessage {

                /**
                 * Constructs a new PullMessage.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.PullMessage.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** PullMessage msgId. */
                msgId: (number|Long);

                /** PullMessage topic. */
                topic: string;

                /** PullMessage senderId. */
                senderId: (number|Long);

                /** PullMessage msgType. */
                msgType: number;

                /** PullMessage content. */
                content: Uint8Array;

                /** PullMessage timestamp. */
                timestamp: (number|Long);

                /** PullMessage topicSeq. */
                topicSeq: (number|Long);

                /** PullMessage clientMsgId. */
                clientMsgId: string;

                /**
                 * Creates a new PullMessage instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns PullMessage instance
                 */
                static create(properties: api.im.v1.PullMessage.$Shape): api.im.v1.PullMessage & api.im.v1.PullMessage.$Shape;
                static create(properties?: api.im.v1.PullMessage.$Properties): api.im.v1.PullMessage;

                /**
                 * Encodes the specified PullMessage message. Does not implicitly {@link api.im.v1.PullMessage.verify|verify} messages.
                 * @param message PullMessage message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.PullMessage.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified PullMessage message, length delimited. Does not implicitly {@link api.im.v1.PullMessage.verify|verify} messages.
                 * @param message PullMessage message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.PullMessage.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a PullMessage message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.PullMessage & api.im.v1.PullMessage.$Shape} PullMessage
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.PullMessage & api.im.v1.PullMessage.$Shape;

                /**
                 * Decodes a PullMessage message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.PullMessage & api.im.v1.PullMessage.$Shape} PullMessage
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.PullMessage & api.im.v1.PullMessage.$Shape;

                /**
                 * Verifies a PullMessage message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a PullMessage message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns PullMessage
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.PullMessage;

                /**
                 * Creates a plain object from a PullMessage message. Also converts values to other types if specified.
                 * @param message PullMessage
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.PullMessage, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this PullMessage to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for PullMessage
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace PullMessage {

                /** Properties of a PullMessage. */
                interface $Properties {

                    /** PullMessage msgId */
                    msgId?: (number|Long|null);

                    /** PullMessage topic */
                    topic?: (string|null);

                    /** PullMessage senderId */
                    senderId?: (number|Long|null);

                    /** PullMessage msgType */
                    msgType?: (number|null);

                    /** PullMessage content */
                    content?: (Uint8Array|null);

                    /** PullMessage timestamp */
                    timestamp?: (number|Long|null);

                    /** PullMessage topicSeq */
                    topicSeq?: (number|Long|null);

                    /** PullMessage clientMsgId */
                    clientMsgId?: (string|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a PullMessage. */
                type $Shape = api.im.v1.PullMessage.$Properties;
            }

            /**
             * Properties of a PullReply.
             * @deprecated Use api.im.v1.PullReply.$Properties instead.
             */
            interface IPullReply extends api.im.v1.PullReply.$Properties {
            }

            /** Represents a PullReply. */
            class PullReply {

                /**
                 * Constructs a new PullReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.PullReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** PullReply messages. */
                messages: api.im.v1.PullMessage.$Properties[];

                /** PullReply hasMore. */
                hasMore: boolean;

                /** PullReply nextSeq. */
                nextSeq: (number|Long);

                /**
                 * Creates a new PullReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns PullReply instance
                 */
                static create(properties: api.im.v1.PullReply.$Shape): api.im.v1.PullReply & api.im.v1.PullReply.$Shape;
                static create(properties?: api.im.v1.PullReply.$Properties): api.im.v1.PullReply;

                /**
                 * Encodes the specified PullReply message. Does not implicitly {@link api.im.v1.PullReply.verify|verify} messages.
                 * @param message PullReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.PullReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified PullReply message, length delimited. Does not implicitly {@link api.im.v1.PullReply.verify|verify} messages.
                 * @param message PullReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.PullReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a PullReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.PullReply & api.im.v1.PullReply.$Shape} PullReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.PullReply & api.im.v1.PullReply.$Shape;

                /**
                 * Decodes a PullReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.PullReply & api.im.v1.PullReply.$Shape} PullReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.PullReply & api.im.v1.PullReply.$Shape;

                /**
                 * Verifies a PullReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a PullReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns PullReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.PullReply;

                /**
                 * Creates a plain object from a PullReply message. Also converts values to other types if specified.
                 * @param message PullReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.PullReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this PullReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for PullReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace PullReply {

                /** Properties of a PullReply. */
                interface $Properties {

                    /** PullReply messages */
                    messages?: (api.im.v1.PullMessage.$Properties[]|null);

                    /** PullReply hasMore */
                    hasMore?: (boolean|null);

                    /** PullReply nextSeq */
                    nextSeq?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a PullReply. */
                type $Shape = api.im.v1.PullReply.$Properties;
            }

            /** Command enum. */
            enum Command {

                /** CMD_UNKNOWN value */
                CMD_UNKNOWN = 0,

                /** CMD_HEARTBEAT value */
                CMD_HEARTBEAT = 1,

                /** CMD_AUTH value */
                CMD_AUTH = 2,

                /** CMD_PUBLISH value */
                CMD_PUBLISH = 3,

                /** CMD_ACK value */
                CMD_ACK = 4,

                /** CMD_PULL value */
                CMD_PULL = 5,

                /** CMD_NOTIFY value */
                CMD_NOTIFY = 6,

                /** CMD_READ_RECEIPT value */
                CMD_READ_RECEIPT = 7,

                /** CMD_DELIVERY_RECEIPT value */
                CMD_DELIVERY_RECEIPT = 8,

                /** CMD_SEND_RECEIPT value */
                CMD_SEND_RECEIPT = 9
            }

            /**
             * Properties of a Packet.
             * @deprecated Use api.im.v1.Packet.$Properties instead.
             */
            interface IPacket extends api.im.v1.Packet.$Properties {
            }

            /** Represents a Packet. */
            class Packet {

                /**
                 * Constructs a new Packet.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.Packet.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** Packet cmd. */
                cmd: api.im.v1.Command;

                /** Packet seq. */
                seq: (number|Long);

                /** Packet authReq. */
                authReq?: (api.im.v1.AuthRequest.$Properties|null);

                /** Packet authResp. */
                authResp?: (api.im.v1.AuthResponse.$Properties|null);

                /** Packet sendReq. */
                sendReq?: (api.im.v1.SendMessageRequest.$Properties|null);

                /** Packet sendReply. */
                sendReply?: (api.im.v1.SendMessageReply.$Properties|null);

                /** Packet pullReq. */
                pullReq?: (api.im.v1.PullRequest.$Properties|null);

                /** Packet pullReply. */
                pullReply?: (api.im.v1.PullReply.$Properties|null);

                /** Packet ackReq. */
                ackReq?: (api.im.v1.AckRequest.$Properties|null);

                /** Packet notify. */
                notify?: (api.im.v1.MessagePush.$Properties|null);

                /** Packet readReceipt. */
                readReceipt?: (api.im.v1.ReadReceipt.$Properties|null);

                /** Packet deliveryReceipt. */
                deliveryReceipt?: (api.im.v1.DeliveryReceipt.$Properties|null);

                /** Packet sendReceipt. */
                sendReceipt?: (api.im.v1.SendReceipt.$Properties|null);

                /** Packet error. */
                error?: (api.im.v1.ErrorResponse.$Properties|null);

                /** Packet payload. */
                payload?: ("authReq"|"authResp"|"sendReq"|"sendReply"|"pullReq"|"pullReply"|"ackReq"|"notify"|"readReceipt"|"deliveryReceipt"|"sendReceipt"|"error");

                /**
                 * Creates a new Packet instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns Packet instance
                 */
                static create(properties: api.im.v1.Packet.$Shape): api.im.v1.Packet & api.im.v1.Packet.$Shape;
                static create(properties?: api.im.v1.Packet.$Properties): api.im.v1.Packet;

                /**
                 * Encodes the specified Packet message. Does not implicitly {@link api.im.v1.Packet.verify|verify} messages.
                 * @param message Packet message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.Packet.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified Packet message, length delimited. Does not implicitly {@link api.im.v1.Packet.verify|verify} messages.
                 * @param message Packet message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.Packet.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a Packet message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.Packet & api.im.v1.Packet.$Shape} Packet
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.Packet & api.im.v1.Packet.$Shape;

                /**
                 * Decodes a Packet message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.Packet & api.im.v1.Packet.$Shape} Packet
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.Packet & api.im.v1.Packet.$Shape;

                /**
                 * Verifies a Packet message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a Packet message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns Packet
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.Packet;

                /**
                 * Creates a plain object from a Packet message. Also converts values to other types if specified.
                 * @param message Packet
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.Packet, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this Packet to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for Packet
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace Packet {

                /** Properties of a Packet. */
                interface $Properties {

                    /** Packet cmd */
                    cmd?: (api.im.v1.Command|null);

                    /** Packet seq */
                    seq?: (number|Long|null);

                    /** Packet authReq */
                    authReq?: (api.im.v1.AuthRequest.$Properties|null);

                    /** Packet authResp */
                    authResp?: (api.im.v1.AuthResponse.$Properties|null);

                    /** Packet sendReq */
                    sendReq?: (api.im.v1.SendMessageRequest.$Properties|null);

                    /** Packet sendReply */
                    sendReply?: (api.im.v1.SendMessageReply.$Properties|null);

                    /** Packet pullReq */
                    pullReq?: (api.im.v1.PullRequest.$Properties|null);

                    /** Packet pullReply */
                    pullReply?: (api.im.v1.PullReply.$Properties|null);

                    /** Packet ackReq */
                    ackReq?: (api.im.v1.AckRequest.$Properties|null);

                    /** Packet notify */
                    notify?: (api.im.v1.MessagePush.$Properties|null);

                    /** Packet readReceipt */
                    readReceipt?: (api.im.v1.ReadReceipt.$Properties|null);

                    /** Packet deliveryReceipt */
                    deliveryReceipt?: (api.im.v1.DeliveryReceipt.$Properties|null);

                    /** Packet sendReceipt */
                    sendReceipt?: (api.im.v1.SendReceipt.$Properties|null);

                    /** Packet error */
                    error?: (api.im.v1.ErrorResponse.$Properties|null);

                    /** Packet payload */
                    payload?: ("authReq"|"authResp"|"sendReq"|"sendReply"|"pullReq"|"pullReply"|"ackReq"|"notify"|"readReceipt"|"deliveryReceipt"|"sendReceipt"|"error");

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Narrowed shape of a Packet. */
                type $Shape = {
                  cmd?: api.im.v1.Command|null;
                  seq?: number|Long|null;
                  authReq?: api.im.v1.AuthRequest.$Shape|null;
                  authResp?: api.im.v1.AuthResponse.$Shape|null;
                  sendReq?: api.im.v1.SendMessageRequest.$Shape|null;
                  sendReply?: api.im.v1.SendMessageReply.$Shape|null;
                  pullReq?: api.im.v1.PullRequest.$Shape|null;
                  pullReply?: api.im.v1.PullReply.$Shape|null;
                  ackReq?: api.im.v1.AckRequest.$Shape|null;
                  notify?: api.im.v1.MessagePush.$Shape|null;
                  readReceipt?: api.im.v1.ReadReceipt.$Shape|null;
                  deliveryReceipt?: api.im.v1.DeliveryReceipt.$Shape|null;
                  sendReceipt?: api.im.v1.SendReceipt.$Shape|null;
                  error?: api.im.v1.ErrorResponse.$Shape|null;
                  $unknowns?: Uint8Array[];
                } & (
                  ({ payload?: undefined; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "authReq"; authReq: api.im.v1.AuthRequest.$Shape; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "authResp"; authReq?: null; authResp: api.im.v1.AuthResponse.$Shape; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "sendReq"; authReq?: null; authResp?: null; sendReq: api.im.v1.SendMessageRequest.$Shape; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "sendReply"; authReq?: null; authResp?: null; sendReq?: null; sendReply: api.im.v1.SendMessageReply.$Shape; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "pullReq"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq: api.im.v1.PullRequest.$Shape; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "pullReply"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply: api.im.v1.PullReply.$Shape; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "ackReq"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq: api.im.v1.AckRequest.$Shape; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "notify"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify: api.im.v1.MessagePush.$Shape; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "readReceipt"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt: api.im.v1.ReadReceipt.$Shape; deliveryReceipt?: null; sendReceipt?: null; error?: null }|{ payload?: "deliveryReceipt"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt: api.im.v1.DeliveryReceipt.$Shape; sendReceipt?: null; error?: null }|{ payload?: "sendReceipt"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt: api.im.v1.SendReceipt.$Shape; error?: null }|{ payload?: "error"; authReq?: null; authResp?: null; sendReq?: null; sendReply?: null; pullReq?: null; pullReply?: null; ackReq?: null; notify?: null; readReceipt?: null; deliveryReceipt?: null; sendReceipt?: null; error: api.im.v1.ErrorResponse.$Shape })
                );
            }

            /**
             * Properties of an AuthRequest.
             * @deprecated Use api.im.v1.AuthRequest.$Properties instead.
             */
            interface IAuthRequest extends api.im.v1.AuthRequest.$Properties {
            }

            /** Represents an AuthRequest. */
            class AuthRequest {

                /**
                 * Constructs a new AuthRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.AuthRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** AuthRequest token. */
                token: string;

                /** AuthRequest deviceId. */
                deviceId: string;

                /**
                 * Creates a new AuthRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns AuthRequest instance
                 */
                static create(properties: api.im.v1.AuthRequest.$Shape): api.im.v1.AuthRequest & api.im.v1.AuthRequest.$Shape;
                static create(properties?: api.im.v1.AuthRequest.$Properties): api.im.v1.AuthRequest;

                /**
                 * Encodes the specified AuthRequest message. Does not implicitly {@link api.im.v1.AuthRequest.verify|verify} messages.
                 * @param message AuthRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.AuthRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified AuthRequest message, length delimited. Does not implicitly {@link api.im.v1.AuthRequest.verify|verify} messages.
                 * @param message AuthRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.AuthRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an AuthRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.AuthRequest & api.im.v1.AuthRequest.$Shape} AuthRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.AuthRequest & api.im.v1.AuthRequest.$Shape;

                /**
                 * Decodes an AuthRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.AuthRequest & api.im.v1.AuthRequest.$Shape} AuthRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.AuthRequest & api.im.v1.AuthRequest.$Shape;

                /**
                 * Verifies an AuthRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an AuthRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns AuthRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.AuthRequest;

                /**
                 * Creates a plain object from an AuthRequest message. Also converts values to other types if specified.
                 * @param message AuthRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.AuthRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this AuthRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for AuthRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace AuthRequest {

                /** Properties of an AuthRequest. */
                interface $Properties {

                    /** AuthRequest token */
                    token?: (string|null);

                    /** AuthRequest deviceId */
                    deviceId?: (string|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an AuthRequest. */
                type $Shape = api.im.v1.AuthRequest.$Properties;
            }

            /**
             * Properties of an AuthResponse.
             * @deprecated Use api.im.v1.AuthResponse.$Properties instead.
             */
            interface IAuthResponse extends api.im.v1.AuthResponse.$Properties {
            }

            /** Represents an AuthResponse. */
            class AuthResponse {

                /**
                 * Constructs a new AuthResponse.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.AuthResponse.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** AuthResponse success. */
                success: boolean;

                /** AuthResponse userId. */
                userId: (number|Long);

                /** AuthResponse deviceId. */
                deviceId: string;

                /**
                 * Creates a new AuthResponse instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns AuthResponse instance
                 */
                static create(properties: api.im.v1.AuthResponse.$Shape): api.im.v1.AuthResponse & api.im.v1.AuthResponse.$Shape;
                static create(properties?: api.im.v1.AuthResponse.$Properties): api.im.v1.AuthResponse;

                /**
                 * Encodes the specified AuthResponse message. Does not implicitly {@link api.im.v1.AuthResponse.verify|verify} messages.
                 * @param message AuthResponse message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.AuthResponse.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified AuthResponse message, length delimited. Does not implicitly {@link api.im.v1.AuthResponse.verify|verify} messages.
                 * @param message AuthResponse message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.AuthResponse.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an AuthResponse message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.AuthResponse & api.im.v1.AuthResponse.$Shape} AuthResponse
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.AuthResponse & api.im.v1.AuthResponse.$Shape;

                /**
                 * Decodes an AuthResponse message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.AuthResponse & api.im.v1.AuthResponse.$Shape} AuthResponse
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.AuthResponse & api.im.v1.AuthResponse.$Shape;

                /**
                 * Verifies an AuthResponse message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an AuthResponse message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns AuthResponse
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.AuthResponse;

                /**
                 * Creates a plain object from an AuthResponse message. Also converts values to other types if specified.
                 * @param message AuthResponse
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.AuthResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this AuthResponse to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for AuthResponse
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace AuthResponse {

                /** Properties of an AuthResponse. */
                interface $Properties {

                    /** AuthResponse success */
                    success?: (boolean|null);

                    /** AuthResponse userId */
                    userId?: (number|Long|null);

                    /** AuthResponse deviceId */
                    deviceId?: (string|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an AuthResponse. */
                type $Shape = api.im.v1.AuthResponse.$Properties;
            }

            /**
             * Properties of an AckRequest.
             * @deprecated Use api.im.v1.AckRequest.$Properties instead.
             */
            interface IAckRequest extends api.im.v1.AckRequest.$Properties {
            }

            /** Represents an AckRequest. */
            class AckRequest {

                /**
                 * Constructs a new AckRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.AckRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** AckRequest msgId. */
                msgId: (number|Long);

                /** AckRequest topic. */
                topic: string;

                /** AckRequest topicSeq. */
                topicSeq: (number|Long);

                /**
                 * Creates a new AckRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns AckRequest instance
                 */
                static create(properties: api.im.v1.AckRequest.$Shape): api.im.v1.AckRequest & api.im.v1.AckRequest.$Shape;
                static create(properties?: api.im.v1.AckRequest.$Properties): api.im.v1.AckRequest;

                /**
                 * Encodes the specified AckRequest message. Does not implicitly {@link api.im.v1.AckRequest.verify|verify} messages.
                 * @param message AckRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.AckRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified AckRequest message, length delimited. Does not implicitly {@link api.im.v1.AckRequest.verify|verify} messages.
                 * @param message AckRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.AckRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an AckRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.AckRequest & api.im.v1.AckRequest.$Shape} AckRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.AckRequest & api.im.v1.AckRequest.$Shape;

                /**
                 * Decodes an AckRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.AckRequest & api.im.v1.AckRequest.$Shape} AckRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.AckRequest & api.im.v1.AckRequest.$Shape;

                /**
                 * Verifies an AckRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an AckRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns AckRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.AckRequest;

                /**
                 * Creates a plain object from an AckRequest message. Also converts values to other types if specified.
                 * @param message AckRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.AckRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this AckRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for AckRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace AckRequest {

                /** Properties of an AckRequest. */
                interface $Properties {

                    /** AckRequest msgId */
                    msgId?: (number|Long|null);

                    /** AckRequest topic */
                    topic?: (string|null);

                    /** AckRequest topicSeq */
                    topicSeq?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an AckRequest. */
                type $Shape = api.im.v1.AckRequest.$Properties;
            }

            /**
             * Properties of an ErrorResponse.
             * @deprecated Use api.im.v1.ErrorResponse.$Properties instead.
             */
            interface IErrorResponse extends api.im.v1.ErrorResponse.$Properties {
            }

            /** Represents an ErrorResponse. */
            class ErrorResponse {

                /**
                 * Constructs a new ErrorResponse.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.ErrorResponse.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** ErrorResponse code. */
                code: number;

                /** ErrorResponse message. */
                message: string;

                /** ErrorResponse retryable. */
                retryable: boolean;

                /** ErrorResponse retryAfterMs. */
                retryAfterMs: (number|Long);

                /**
                 * Creates a new ErrorResponse instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns ErrorResponse instance
                 */
                static create(properties: api.im.v1.ErrorResponse.$Shape): api.im.v1.ErrorResponse & api.im.v1.ErrorResponse.$Shape;
                static create(properties?: api.im.v1.ErrorResponse.$Properties): api.im.v1.ErrorResponse;

                /**
                 * Encodes the specified ErrorResponse message. Does not implicitly {@link api.im.v1.ErrorResponse.verify|verify} messages.
                 * @param message ErrorResponse message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.ErrorResponse.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified ErrorResponse message, length delimited. Does not implicitly {@link api.im.v1.ErrorResponse.verify|verify} messages.
                 * @param message ErrorResponse message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.ErrorResponse.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an ErrorResponse message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.ErrorResponse & api.im.v1.ErrorResponse.$Shape} ErrorResponse
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.ErrorResponse & api.im.v1.ErrorResponse.$Shape;

                /**
                 * Decodes an ErrorResponse message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.ErrorResponse & api.im.v1.ErrorResponse.$Shape} ErrorResponse
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.ErrorResponse & api.im.v1.ErrorResponse.$Shape;

                /**
                 * Verifies an ErrorResponse message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an ErrorResponse message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns ErrorResponse
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.ErrorResponse;

                /**
                 * Creates a plain object from an ErrorResponse message. Also converts values to other types if specified.
                 * @param message ErrorResponse
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.ErrorResponse, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this ErrorResponse to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for ErrorResponse
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace ErrorResponse {

                /** Properties of an ErrorResponse. */
                interface $Properties {

                    /** ErrorResponse code */
                    code?: (number|null);

                    /** ErrorResponse message */
                    message?: (string|null);

                    /** ErrorResponse retryable */
                    retryable?: (boolean|null);

                    /** ErrorResponse retryAfterMs */
                    retryAfterMs?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an ErrorResponse. */
                type $Shape = api.im.v1.ErrorResponse.$Properties;
            }

            /**
             * Properties of a ReadReceipt.
             * @deprecated Use api.im.v1.ReadReceipt.$Properties instead.
             */
            interface IReadReceipt extends api.im.v1.ReadReceipt.$Properties {
            }

            /** Represents a ReadReceipt. */
            class ReadReceipt {

                /**
                 * Constructs a new ReadReceipt.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.ReadReceipt.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** ReadReceipt topic. */
                topic: string;

                /** ReadReceipt upToSeq. */
                upToSeq: (number|Long);

                /** ReadReceipt readerId. */
                readerId: (number|Long);

                /**
                 * Creates a new ReadReceipt instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns ReadReceipt instance
                 */
                static create(properties: api.im.v1.ReadReceipt.$Shape): api.im.v1.ReadReceipt & api.im.v1.ReadReceipt.$Shape;
                static create(properties?: api.im.v1.ReadReceipt.$Properties): api.im.v1.ReadReceipt;

                /**
                 * Encodes the specified ReadReceipt message. Does not implicitly {@link api.im.v1.ReadReceipt.verify|verify} messages.
                 * @param message ReadReceipt message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.ReadReceipt.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified ReadReceipt message, length delimited. Does not implicitly {@link api.im.v1.ReadReceipt.verify|verify} messages.
                 * @param message ReadReceipt message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.ReadReceipt.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a ReadReceipt message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.ReadReceipt & api.im.v1.ReadReceipt.$Shape} ReadReceipt
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.ReadReceipt & api.im.v1.ReadReceipt.$Shape;

                /**
                 * Decodes a ReadReceipt message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.ReadReceipt & api.im.v1.ReadReceipt.$Shape} ReadReceipt
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.ReadReceipt & api.im.v1.ReadReceipt.$Shape;

                /**
                 * Verifies a ReadReceipt message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a ReadReceipt message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns ReadReceipt
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.ReadReceipt;

                /**
                 * Creates a plain object from a ReadReceipt message. Also converts values to other types if specified.
                 * @param message ReadReceipt
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.ReadReceipt, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this ReadReceipt to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for ReadReceipt
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace ReadReceipt {

                /** Properties of a ReadReceipt. */
                interface $Properties {

                    /** ReadReceipt topic */
                    topic?: (string|null);

                    /** ReadReceipt upToSeq */
                    upToSeq?: (number|Long|null);

                    /** ReadReceipt readerId */
                    readerId?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a ReadReceipt. */
                type $Shape = api.im.v1.ReadReceipt.$Properties;
            }

            /**
             * Properties of a DeliveryReceipt.
             * @deprecated Use api.im.v1.DeliveryReceipt.$Properties instead.
             */
            interface IDeliveryReceipt extends api.im.v1.DeliveryReceipt.$Properties {
            }

            /** Represents a DeliveryReceipt. */
            class DeliveryReceipt {

                /**
                 * Constructs a new DeliveryReceipt.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.DeliveryReceipt.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** DeliveryReceipt topic. */
                topic: string;

                /** DeliveryReceipt topicSeq. */
                topicSeq: (number|Long);

                /** DeliveryReceipt msgId. */
                msgId: (number|Long);

                /**
                 * Creates a new DeliveryReceipt instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns DeliveryReceipt instance
                 */
                static create(properties: api.im.v1.DeliveryReceipt.$Shape): api.im.v1.DeliveryReceipt & api.im.v1.DeliveryReceipt.$Shape;
                static create(properties?: api.im.v1.DeliveryReceipt.$Properties): api.im.v1.DeliveryReceipt;

                /**
                 * Encodes the specified DeliveryReceipt message. Does not implicitly {@link api.im.v1.DeliveryReceipt.verify|verify} messages.
                 * @param message DeliveryReceipt message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.DeliveryReceipt.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified DeliveryReceipt message, length delimited. Does not implicitly {@link api.im.v1.DeliveryReceipt.verify|verify} messages.
                 * @param message DeliveryReceipt message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.DeliveryReceipt.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a DeliveryReceipt message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.DeliveryReceipt & api.im.v1.DeliveryReceipt.$Shape} DeliveryReceipt
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.DeliveryReceipt & api.im.v1.DeliveryReceipt.$Shape;

                /**
                 * Decodes a DeliveryReceipt message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.DeliveryReceipt & api.im.v1.DeliveryReceipt.$Shape} DeliveryReceipt
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.DeliveryReceipt & api.im.v1.DeliveryReceipt.$Shape;

                /**
                 * Verifies a DeliveryReceipt message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a DeliveryReceipt message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns DeliveryReceipt
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.DeliveryReceipt;

                /**
                 * Creates a plain object from a DeliveryReceipt message. Also converts values to other types if specified.
                 * @param message DeliveryReceipt
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.DeliveryReceipt, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this DeliveryReceipt to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for DeliveryReceipt
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace DeliveryReceipt {

                /** Properties of a DeliveryReceipt. */
                interface $Properties {

                    /** DeliveryReceipt topic */
                    topic?: (string|null);

                    /** DeliveryReceipt topicSeq */
                    topicSeq?: (number|Long|null);

                    /** DeliveryReceipt msgId */
                    msgId?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a DeliveryReceipt. */
                type $Shape = api.im.v1.DeliveryReceipt.$Properties;
            }

            /**
             * Properties of a SendReceipt.
             * @deprecated Use api.im.v1.SendReceipt.$Properties instead.
             */
            interface ISendReceipt extends api.im.v1.SendReceipt.$Properties {
            }

            /** Represents a SendReceipt. */
            class SendReceipt {

                /**
                 * Constructs a new SendReceipt.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.SendReceipt.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** SendReceipt clientMsgId. */
                clientMsgId: string;

                /** SendReceipt msgId. */
                msgId: (number|Long);

                /** SendReceipt topic. */
                topic: string;

                /** SendReceipt topicSeq. */
                topicSeq: (number|Long);

                /** SendReceipt timestamp. */
                timestamp: (number|Long);

                /**
                 * Creates a new SendReceipt instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns SendReceipt instance
                 */
                static create(properties: api.im.v1.SendReceipt.$Shape): api.im.v1.SendReceipt & api.im.v1.SendReceipt.$Shape;
                static create(properties?: api.im.v1.SendReceipt.$Properties): api.im.v1.SendReceipt;

                /**
                 * Encodes the specified SendReceipt message. Does not implicitly {@link api.im.v1.SendReceipt.verify|verify} messages.
                 * @param message SendReceipt message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.SendReceipt.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified SendReceipt message, length delimited. Does not implicitly {@link api.im.v1.SendReceipt.verify|verify} messages.
                 * @param message SendReceipt message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.SendReceipt.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a SendReceipt message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.SendReceipt & api.im.v1.SendReceipt.$Shape} SendReceipt
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.SendReceipt & api.im.v1.SendReceipt.$Shape;

                /**
                 * Decodes a SendReceipt message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.SendReceipt & api.im.v1.SendReceipt.$Shape} SendReceipt
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.SendReceipt & api.im.v1.SendReceipt.$Shape;

                /**
                 * Verifies a SendReceipt message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a SendReceipt message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns SendReceipt
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.SendReceipt;

                /**
                 * Creates a plain object from a SendReceipt message. Also converts values to other types if specified.
                 * @param message SendReceipt
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.SendReceipt, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this SendReceipt to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for SendReceipt
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace SendReceipt {

                /** Properties of a SendReceipt. */
                interface $Properties {

                    /** SendReceipt clientMsgId */
                    clientMsgId?: (string|null);

                    /** SendReceipt msgId */
                    msgId?: (number|Long|null);

                    /** SendReceipt topic */
                    topic?: (string|null);

                    /** SendReceipt topicSeq */
                    topicSeq?: (number|Long|null);

                    /** SendReceipt timestamp */
                    timestamp?: (number|Long|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a SendReceipt. */
                type $Shape = api.im.v1.SendReceipt.$Properties;
            }

            /** Represents a PushService */
            class PushService extends $protobuf.rpc.Service {

                /**
                 * Constructs a new PushService service.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 */
                constructor(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean);

                /**
                 * Creates new PushService service using the specified rpc implementation.
                 * @param rpcImpl RPC implementation
                 * @param [requestDelimited=false] Whether requests are length-delimited
                 * @param [responseDelimited=false] Whether responses are length-delimited
                 * @returns RPC service. Useful where requests and/or responses are streamed.
                 */
                static create(rpcImpl: $protobuf.RPCImpl, requestDelimited?: boolean, responseDelimited?: boolean): PushService;

                /** Calls PushToUser. */
                pushToUser: api.im.v1.PushService.PushToUser;

                /** Calls BatchPushToUsers. */
                batchPushToUsers: api.im.v1.PushService.BatchPushToUsers;

                /** Calls PushReceiptToUser. */
                pushReceiptToUser: api.im.v1.PushService.PushReceiptToUser;

                /** Calls BatchPushReceiptToUsers. */
                batchPushReceiptToUsers: api.im.v1.PushService.BatchPushReceiptToUsers;

                /** Calls BatchPushReceiptsToUsers. */
                batchPushReceiptsToUsers: api.im.v1.PushService.BatchPushReceiptsToUsers;
            }

            namespace PushService {

                /**
                 * Callback as used by {@link api.im.v1.PushService#pushToUser}.
                 * @param error Error, if any
                 * @param [response] PushToUserReply
                 */
                type PushToUserCallback = (error: (Error|null), response?: api.im.v1.PushToUserReply) => void;

                /** Calls PushToUser. */
                type PushToUser = {
                  (request: api.im.v1.IPushToUserRequest, callback: api.im.v1.PushService.PushToUserCallback): void;
                  (request: api.im.v1.IPushToUserRequest): Promise<api.im.v1.PushToUserReply>;
                  readonly name: "PushToUser";
                  readonly path: "/api.im.v1.PushService/PushToUser";
                  readonly requestType: "PushToUserRequest";
                  readonly responseType: "PushToUserReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };

                /**
                 * Callback as used by {@link api.im.v1.PushService#batchPushToUsers}.
                 * @param error Error, if any
                 * @param [response] BatchPushToUsersReply
                 */
                type BatchPushToUsersCallback = (error: (Error|null), response?: api.im.v1.BatchPushToUsersReply) => void;

                /** Calls BatchPushToUsers. */
                type BatchPushToUsers = {
                  (request: api.im.v1.IBatchPushToUsersRequest, callback: api.im.v1.PushService.BatchPushToUsersCallback): void;
                  (request: api.im.v1.IBatchPushToUsersRequest): Promise<api.im.v1.BatchPushToUsersReply>;
                  readonly name: "BatchPushToUsers";
                  readonly path: "/api.im.v1.PushService/BatchPushToUsers";
                  readonly requestType: "BatchPushToUsersRequest";
                  readonly responseType: "BatchPushToUsersReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };

                /**
                 * Callback as used by {@link api.im.v1.PushService#pushReceiptToUser}.
                 * @param error Error, if any
                 * @param [response] PushReceiptToUserReply
                 */
                type PushReceiptToUserCallback = (error: (Error|null), response?: api.im.v1.PushReceiptToUserReply) => void;

                /** Calls PushReceiptToUser. */
                type PushReceiptToUser = {
                  (request: api.im.v1.IPushReceiptToUserRequest, callback: api.im.v1.PushService.PushReceiptToUserCallback): void;
                  (request: api.im.v1.IPushReceiptToUserRequest): Promise<api.im.v1.PushReceiptToUserReply>;
                  readonly name: "PushReceiptToUser";
                  readonly path: "/api.im.v1.PushService/PushReceiptToUser";
                  readonly requestType: "PushReceiptToUserRequest";
                  readonly responseType: "PushReceiptToUserReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };

                /**
                 * Callback as used by {@link api.im.v1.PushService#batchPushReceiptToUsers}.
                 * @param error Error, if any
                 * @param [response] BatchPushReceiptToUsersReply
                 */
                type BatchPushReceiptToUsersCallback = (error: (Error|null), response?: api.im.v1.BatchPushReceiptToUsersReply) => void;

                /** Calls BatchPushReceiptToUsers. */
                type BatchPushReceiptToUsers = {
                  (request: api.im.v1.IBatchPushReceiptToUsersRequest, callback: api.im.v1.PushService.BatchPushReceiptToUsersCallback): void;
                  (request: api.im.v1.IBatchPushReceiptToUsersRequest): Promise<api.im.v1.BatchPushReceiptToUsersReply>;
                  readonly name: "BatchPushReceiptToUsers";
                  readonly path: "/api.im.v1.PushService/BatchPushReceiptToUsers";
                  readonly requestType: "BatchPushReceiptToUsersRequest";
                  readonly responseType: "BatchPushReceiptToUsersReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };

                /**
                 * Callback as used by {@link api.im.v1.PushService#batchPushReceiptsToUsers}.
                 * @param error Error, if any
                 * @param [response] BatchPushReceiptsToUsersReply
                 */
                type BatchPushReceiptsToUsersCallback = (error: (Error|null), response?: api.im.v1.BatchPushReceiptsToUsersReply) => void;

                /** Calls BatchPushReceiptsToUsers. */
                type BatchPushReceiptsToUsers = {
                  (request: api.im.v1.IBatchPushReceiptsToUsersRequest, callback: api.im.v1.PushService.BatchPushReceiptsToUsersCallback): void;
                  (request: api.im.v1.IBatchPushReceiptsToUsersRequest): Promise<api.im.v1.BatchPushReceiptsToUsersReply>;
                  readonly name: "BatchPushReceiptsToUsers";
                  readonly path: "/api.im.v1.PushService/BatchPushReceiptsToUsers";
                  readonly requestType: "BatchPushReceiptsToUsersRequest";
                  readonly responseType: "BatchPushReceiptsToUsersReply";
                  readonly requestStream: undefined;
                  readonly responseStream: undefined;
                };
            }

            /**
             * Properties of a PushToUserRequest.
             * @deprecated Use api.im.v1.PushToUserRequest.$Properties instead.
             */
            interface IPushToUserRequest extends api.im.v1.PushToUserRequest.$Properties {
            }

            /** Represents a PushToUserRequest. */
            class PushToUserRequest {

                /**
                 * Constructs a new PushToUserRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.PushToUserRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** PushToUserRequest userId. */
                userId: (number|Long);

                /** PushToUserRequest message. */
                message?: (api.im.v1.MessagePush.$Properties|null);

                /**
                 * Creates a new PushToUserRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns PushToUserRequest instance
                 */
                static create(properties: api.im.v1.PushToUserRequest.$Shape): api.im.v1.PushToUserRequest & api.im.v1.PushToUserRequest.$Shape;
                static create(properties?: api.im.v1.PushToUserRequest.$Properties): api.im.v1.PushToUserRequest;

                /**
                 * Encodes the specified PushToUserRequest message. Does not implicitly {@link api.im.v1.PushToUserRequest.verify|verify} messages.
                 * @param message PushToUserRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.PushToUserRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified PushToUserRequest message, length delimited. Does not implicitly {@link api.im.v1.PushToUserRequest.verify|verify} messages.
                 * @param message PushToUserRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.PushToUserRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a PushToUserRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.PushToUserRequest & api.im.v1.PushToUserRequest.$Shape} PushToUserRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.PushToUserRequest & api.im.v1.PushToUserRequest.$Shape;

                /**
                 * Decodes a PushToUserRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.PushToUserRequest & api.im.v1.PushToUserRequest.$Shape} PushToUserRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.PushToUserRequest & api.im.v1.PushToUserRequest.$Shape;

                /**
                 * Verifies a PushToUserRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a PushToUserRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns PushToUserRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.PushToUserRequest;

                /**
                 * Creates a plain object from a PushToUserRequest message. Also converts values to other types if specified.
                 * @param message PushToUserRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.PushToUserRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this PushToUserRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for PushToUserRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace PushToUserRequest {

                /** Properties of a PushToUserRequest. */
                interface $Properties {

                    /** PushToUserRequest userId */
                    userId?: (number|Long|null);

                    /** PushToUserRequest message */
                    message?: (api.im.v1.MessagePush.$Properties|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a PushToUserRequest. */
                type $Shape = api.im.v1.PushToUserRequest.$Properties;
            }

            /**
             * Properties of a PushToUserReply.
             * @deprecated Use api.im.v1.PushToUserReply.$Properties instead.
             */
            interface IPushToUserReply extends api.im.v1.PushToUserReply.$Properties {
            }

            /** Represents a PushToUserReply. */
            class PushToUserReply {

                /**
                 * Constructs a new PushToUserReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.PushToUserReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** PushToUserReply success. */
                success: boolean;

                /** PushToUserReply deliveredCount. */
                deliveredCount: number;

                /**
                 * Creates a new PushToUserReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns PushToUserReply instance
                 */
                static create(properties: api.im.v1.PushToUserReply.$Shape): api.im.v1.PushToUserReply & api.im.v1.PushToUserReply.$Shape;
                static create(properties?: api.im.v1.PushToUserReply.$Properties): api.im.v1.PushToUserReply;

                /**
                 * Encodes the specified PushToUserReply message. Does not implicitly {@link api.im.v1.PushToUserReply.verify|verify} messages.
                 * @param message PushToUserReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.PushToUserReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified PushToUserReply message, length delimited. Does not implicitly {@link api.im.v1.PushToUserReply.verify|verify} messages.
                 * @param message PushToUserReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.PushToUserReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a PushToUserReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.PushToUserReply & api.im.v1.PushToUserReply.$Shape} PushToUserReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.PushToUserReply & api.im.v1.PushToUserReply.$Shape;

                /**
                 * Decodes a PushToUserReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.PushToUserReply & api.im.v1.PushToUserReply.$Shape} PushToUserReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.PushToUserReply & api.im.v1.PushToUserReply.$Shape;

                /**
                 * Verifies a PushToUserReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a PushToUserReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns PushToUserReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.PushToUserReply;

                /**
                 * Creates a plain object from a PushToUserReply message. Also converts values to other types if specified.
                 * @param message PushToUserReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.PushToUserReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this PushToUserReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for PushToUserReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace PushToUserReply {

                /** Properties of a PushToUserReply. */
                interface $Properties {

                    /** PushToUserReply success */
                    success?: (boolean|null);

                    /** PushToUserReply deliveredCount */
                    deliveredCount?: (number|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a PushToUserReply. */
                type $Shape = api.im.v1.PushToUserReply.$Properties;
            }

            /**
             * Properties of a BatchPushToUsersRequest.
             * @deprecated Use api.im.v1.BatchPushToUsersRequest.$Properties instead.
             */
            interface IBatchPushToUsersRequest extends api.im.v1.BatchPushToUsersRequest.$Properties {
            }

            /** Represents a BatchPushToUsersRequest. */
            class BatchPushToUsersRequest {

                /**
                 * Constructs a new BatchPushToUsersRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.BatchPushToUsersRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** BatchPushToUsersRequest userIds. */
                userIds: (number|Long)[];

                /** BatchPushToUsersRequest message. */
                message?: (api.im.v1.MessagePush.$Properties|null);

                /**
                 * Creates a new BatchPushToUsersRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns BatchPushToUsersRequest instance
                 */
                static create(properties: api.im.v1.BatchPushToUsersRequest.$Shape): api.im.v1.BatchPushToUsersRequest & api.im.v1.BatchPushToUsersRequest.$Shape;
                static create(properties?: api.im.v1.BatchPushToUsersRequest.$Properties): api.im.v1.BatchPushToUsersRequest;

                /**
                 * Encodes the specified BatchPushToUsersRequest message. Does not implicitly {@link api.im.v1.BatchPushToUsersRequest.verify|verify} messages.
                 * @param message BatchPushToUsersRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.BatchPushToUsersRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified BatchPushToUsersRequest message, length delimited. Does not implicitly {@link api.im.v1.BatchPushToUsersRequest.verify|verify} messages.
                 * @param message BatchPushToUsersRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.BatchPushToUsersRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a BatchPushToUsersRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.BatchPushToUsersRequest & api.im.v1.BatchPushToUsersRequest.$Shape} BatchPushToUsersRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.BatchPushToUsersRequest & api.im.v1.BatchPushToUsersRequest.$Shape;

                /**
                 * Decodes a BatchPushToUsersRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.BatchPushToUsersRequest & api.im.v1.BatchPushToUsersRequest.$Shape} BatchPushToUsersRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.BatchPushToUsersRequest & api.im.v1.BatchPushToUsersRequest.$Shape;

                /**
                 * Verifies a BatchPushToUsersRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a BatchPushToUsersRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns BatchPushToUsersRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.BatchPushToUsersRequest;

                /**
                 * Creates a plain object from a BatchPushToUsersRequest message. Also converts values to other types if specified.
                 * @param message BatchPushToUsersRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.BatchPushToUsersRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this BatchPushToUsersRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for BatchPushToUsersRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace BatchPushToUsersRequest {

                /** Properties of a BatchPushToUsersRequest. */
                interface $Properties {

                    /** BatchPushToUsersRequest userIds */
                    userIds?: ((number|Long)[]|null);

                    /** BatchPushToUsersRequest message */
                    message?: (api.im.v1.MessagePush.$Properties|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a BatchPushToUsersRequest. */
                type $Shape = api.im.v1.BatchPushToUsersRequest.$Properties;
            }

            /**
             * Properties of a BatchPushToUsersReply.
             * @deprecated Use api.im.v1.BatchPushToUsersReply.$Properties instead.
             */
            interface IBatchPushToUsersReply extends api.im.v1.BatchPushToUsersReply.$Properties {
            }

            /** Represents a BatchPushToUsersReply. */
            class BatchPushToUsersReply {

                /**
                 * Constructs a new BatchPushToUsersReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.BatchPushToUsersReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** BatchPushToUsersReply totalDelivered. */
                totalDelivered: number;

                /** BatchPushToUsersReply failedUserIds. */
                failedUserIds: (number|Long)[];

                /**
                 * Creates a new BatchPushToUsersReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns BatchPushToUsersReply instance
                 */
                static create(properties: api.im.v1.BatchPushToUsersReply.$Shape): api.im.v1.BatchPushToUsersReply & api.im.v1.BatchPushToUsersReply.$Shape;
                static create(properties?: api.im.v1.BatchPushToUsersReply.$Properties): api.im.v1.BatchPushToUsersReply;

                /**
                 * Encodes the specified BatchPushToUsersReply message. Does not implicitly {@link api.im.v1.BatchPushToUsersReply.verify|verify} messages.
                 * @param message BatchPushToUsersReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.BatchPushToUsersReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified BatchPushToUsersReply message, length delimited. Does not implicitly {@link api.im.v1.BatchPushToUsersReply.verify|verify} messages.
                 * @param message BatchPushToUsersReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.BatchPushToUsersReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a BatchPushToUsersReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.BatchPushToUsersReply & api.im.v1.BatchPushToUsersReply.$Shape} BatchPushToUsersReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.BatchPushToUsersReply & api.im.v1.BatchPushToUsersReply.$Shape;

                /**
                 * Decodes a BatchPushToUsersReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.BatchPushToUsersReply & api.im.v1.BatchPushToUsersReply.$Shape} BatchPushToUsersReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.BatchPushToUsersReply & api.im.v1.BatchPushToUsersReply.$Shape;

                /**
                 * Verifies a BatchPushToUsersReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a BatchPushToUsersReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns BatchPushToUsersReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.BatchPushToUsersReply;

                /**
                 * Creates a plain object from a BatchPushToUsersReply message. Also converts values to other types if specified.
                 * @param message BatchPushToUsersReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.BatchPushToUsersReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this BatchPushToUsersReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for BatchPushToUsersReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace BatchPushToUsersReply {

                /** Properties of a BatchPushToUsersReply. */
                interface $Properties {

                    /** BatchPushToUsersReply totalDelivered */
                    totalDelivered?: (number|null);

                    /** BatchPushToUsersReply failedUserIds */
                    failedUserIds?: ((number|Long)[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a BatchPushToUsersReply. */
                type $Shape = api.im.v1.BatchPushToUsersReply.$Properties;
            }

            /**
             * Properties of a PushReceiptToUserRequest.
             * @deprecated Use api.im.v1.PushReceiptToUserRequest.$Properties instead.
             */
            interface IPushReceiptToUserRequest extends api.im.v1.PushReceiptToUserRequest.$Properties {
            }

            /** Represents a PushReceiptToUserRequest. */
            class PushReceiptToUserRequest {

                /**
                 * Constructs a new PushReceiptToUserRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.PushReceiptToUserRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** PushReceiptToUserRequest userId. */
                userId: (number|Long);

                /** PushReceiptToUserRequest receipt. */
                receipt?: (api.im.v1.SendReceipt.$Properties|null);

                /**
                 * Creates a new PushReceiptToUserRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns PushReceiptToUserRequest instance
                 */
                static create(properties: api.im.v1.PushReceiptToUserRequest.$Shape): api.im.v1.PushReceiptToUserRequest & api.im.v1.PushReceiptToUserRequest.$Shape;
                static create(properties?: api.im.v1.PushReceiptToUserRequest.$Properties): api.im.v1.PushReceiptToUserRequest;

                /**
                 * Encodes the specified PushReceiptToUserRequest message. Does not implicitly {@link api.im.v1.PushReceiptToUserRequest.verify|verify} messages.
                 * @param message PushReceiptToUserRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.PushReceiptToUserRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified PushReceiptToUserRequest message, length delimited. Does not implicitly {@link api.im.v1.PushReceiptToUserRequest.verify|verify} messages.
                 * @param message PushReceiptToUserRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.PushReceiptToUserRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a PushReceiptToUserRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.PushReceiptToUserRequest & api.im.v1.PushReceiptToUserRequest.$Shape} PushReceiptToUserRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.PushReceiptToUserRequest & api.im.v1.PushReceiptToUserRequest.$Shape;

                /**
                 * Decodes a PushReceiptToUserRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.PushReceiptToUserRequest & api.im.v1.PushReceiptToUserRequest.$Shape} PushReceiptToUserRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.PushReceiptToUserRequest & api.im.v1.PushReceiptToUserRequest.$Shape;

                /**
                 * Verifies a PushReceiptToUserRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a PushReceiptToUserRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns PushReceiptToUserRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.PushReceiptToUserRequest;

                /**
                 * Creates a plain object from a PushReceiptToUserRequest message. Also converts values to other types if specified.
                 * @param message PushReceiptToUserRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.PushReceiptToUserRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this PushReceiptToUserRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for PushReceiptToUserRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace PushReceiptToUserRequest {

                /** Properties of a PushReceiptToUserRequest. */
                interface $Properties {

                    /** PushReceiptToUserRequest userId */
                    userId?: (number|Long|null);

                    /** PushReceiptToUserRequest receipt */
                    receipt?: (api.im.v1.SendReceipt.$Properties|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a PushReceiptToUserRequest. */
                type $Shape = api.im.v1.PushReceiptToUserRequest.$Properties;
            }

            /**
             * Properties of a PushReceiptToUserReply.
             * @deprecated Use api.im.v1.PushReceiptToUserReply.$Properties instead.
             */
            interface IPushReceiptToUserReply extends api.im.v1.PushReceiptToUserReply.$Properties {
            }

            /** Represents a PushReceiptToUserReply. */
            class PushReceiptToUserReply {

                /**
                 * Constructs a new PushReceiptToUserReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.PushReceiptToUserReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** PushReceiptToUserReply success. */
                success: boolean;

                /** PushReceiptToUserReply deliveredCount. */
                deliveredCount: number;

                /**
                 * Creates a new PushReceiptToUserReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns PushReceiptToUserReply instance
                 */
                static create(properties: api.im.v1.PushReceiptToUserReply.$Shape): api.im.v1.PushReceiptToUserReply & api.im.v1.PushReceiptToUserReply.$Shape;
                static create(properties?: api.im.v1.PushReceiptToUserReply.$Properties): api.im.v1.PushReceiptToUserReply;

                /**
                 * Encodes the specified PushReceiptToUserReply message. Does not implicitly {@link api.im.v1.PushReceiptToUserReply.verify|verify} messages.
                 * @param message PushReceiptToUserReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.PushReceiptToUserReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified PushReceiptToUserReply message, length delimited. Does not implicitly {@link api.im.v1.PushReceiptToUserReply.verify|verify} messages.
                 * @param message PushReceiptToUserReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.PushReceiptToUserReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a PushReceiptToUserReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.PushReceiptToUserReply & api.im.v1.PushReceiptToUserReply.$Shape} PushReceiptToUserReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.PushReceiptToUserReply & api.im.v1.PushReceiptToUserReply.$Shape;

                /**
                 * Decodes a PushReceiptToUserReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.PushReceiptToUserReply & api.im.v1.PushReceiptToUserReply.$Shape} PushReceiptToUserReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.PushReceiptToUserReply & api.im.v1.PushReceiptToUserReply.$Shape;

                /**
                 * Verifies a PushReceiptToUserReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a PushReceiptToUserReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns PushReceiptToUserReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.PushReceiptToUserReply;

                /**
                 * Creates a plain object from a PushReceiptToUserReply message. Also converts values to other types if specified.
                 * @param message PushReceiptToUserReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.PushReceiptToUserReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this PushReceiptToUserReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for PushReceiptToUserReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace PushReceiptToUserReply {

                /** Properties of a PushReceiptToUserReply. */
                interface $Properties {

                    /** PushReceiptToUserReply success */
                    success?: (boolean|null);

                    /** PushReceiptToUserReply deliveredCount */
                    deliveredCount?: (number|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a PushReceiptToUserReply. */
                type $Shape = api.im.v1.PushReceiptToUserReply.$Properties;
            }

            /**
             * Properties of a BatchPushReceiptToUsersRequest.
             * @deprecated Use api.im.v1.BatchPushReceiptToUsersRequest.$Properties instead.
             */
            interface IBatchPushReceiptToUsersRequest extends api.im.v1.BatchPushReceiptToUsersRequest.$Properties {
            }

            /** Represents a BatchPushReceiptToUsersRequest. */
            class BatchPushReceiptToUsersRequest {

                /**
                 * Constructs a new BatchPushReceiptToUsersRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.BatchPushReceiptToUsersRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** BatchPushReceiptToUsersRequest userIds. */
                userIds: (number|Long)[];

                /** BatchPushReceiptToUsersRequest receipt. */
                receipt?: (api.im.v1.SendReceipt.$Properties|null);

                /**
                 * Creates a new BatchPushReceiptToUsersRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns BatchPushReceiptToUsersRequest instance
                 */
                static create(properties: api.im.v1.BatchPushReceiptToUsersRequest.$Shape): api.im.v1.BatchPushReceiptToUsersRequest & api.im.v1.BatchPushReceiptToUsersRequest.$Shape;
                static create(properties?: api.im.v1.BatchPushReceiptToUsersRequest.$Properties): api.im.v1.BatchPushReceiptToUsersRequest;

                /**
                 * Encodes the specified BatchPushReceiptToUsersRequest message. Does not implicitly {@link api.im.v1.BatchPushReceiptToUsersRequest.verify|verify} messages.
                 * @param message BatchPushReceiptToUsersRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.BatchPushReceiptToUsersRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified BatchPushReceiptToUsersRequest message, length delimited. Does not implicitly {@link api.im.v1.BatchPushReceiptToUsersRequest.verify|verify} messages.
                 * @param message BatchPushReceiptToUsersRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.BatchPushReceiptToUsersRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a BatchPushReceiptToUsersRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.BatchPushReceiptToUsersRequest & api.im.v1.BatchPushReceiptToUsersRequest.$Shape} BatchPushReceiptToUsersRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.BatchPushReceiptToUsersRequest & api.im.v1.BatchPushReceiptToUsersRequest.$Shape;

                /**
                 * Decodes a BatchPushReceiptToUsersRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.BatchPushReceiptToUsersRequest & api.im.v1.BatchPushReceiptToUsersRequest.$Shape} BatchPushReceiptToUsersRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.BatchPushReceiptToUsersRequest & api.im.v1.BatchPushReceiptToUsersRequest.$Shape;

                /**
                 * Verifies a BatchPushReceiptToUsersRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a BatchPushReceiptToUsersRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns BatchPushReceiptToUsersRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.BatchPushReceiptToUsersRequest;

                /**
                 * Creates a plain object from a BatchPushReceiptToUsersRequest message. Also converts values to other types if specified.
                 * @param message BatchPushReceiptToUsersRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.BatchPushReceiptToUsersRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this BatchPushReceiptToUsersRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for BatchPushReceiptToUsersRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace BatchPushReceiptToUsersRequest {

                /** Properties of a BatchPushReceiptToUsersRequest. */
                interface $Properties {

                    /** BatchPushReceiptToUsersRequest userIds */
                    userIds?: ((number|Long)[]|null);

                    /** BatchPushReceiptToUsersRequest receipt */
                    receipt?: (api.im.v1.SendReceipt.$Properties|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a BatchPushReceiptToUsersRequest. */
                type $Shape = api.im.v1.BatchPushReceiptToUsersRequest.$Properties;
            }

            /**
             * Properties of a BatchPushReceiptToUsersReply.
             * @deprecated Use api.im.v1.BatchPushReceiptToUsersReply.$Properties instead.
             */
            interface IBatchPushReceiptToUsersReply extends api.im.v1.BatchPushReceiptToUsersReply.$Properties {
            }

            /** Represents a BatchPushReceiptToUsersReply. */
            class BatchPushReceiptToUsersReply {

                /**
                 * Constructs a new BatchPushReceiptToUsersReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.BatchPushReceiptToUsersReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** BatchPushReceiptToUsersReply totalDelivered. */
                totalDelivered: number;

                /** BatchPushReceiptToUsersReply failedUserIds. */
                failedUserIds: (number|Long)[];

                /**
                 * Creates a new BatchPushReceiptToUsersReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns BatchPushReceiptToUsersReply instance
                 */
                static create(properties: api.im.v1.BatchPushReceiptToUsersReply.$Shape): api.im.v1.BatchPushReceiptToUsersReply & api.im.v1.BatchPushReceiptToUsersReply.$Shape;
                static create(properties?: api.im.v1.BatchPushReceiptToUsersReply.$Properties): api.im.v1.BatchPushReceiptToUsersReply;

                /**
                 * Encodes the specified BatchPushReceiptToUsersReply message. Does not implicitly {@link api.im.v1.BatchPushReceiptToUsersReply.verify|verify} messages.
                 * @param message BatchPushReceiptToUsersReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.BatchPushReceiptToUsersReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified BatchPushReceiptToUsersReply message, length delimited. Does not implicitly {@link api.im.v1.BatchPushReceiptToUsersReply.verify|verify} messages.
                 * @param message BatchPushReceiptToUsersReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.BatchPushReceiptToUsersReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a BatchPushReceiptToUsersReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.BatchPushReceiptToUsersReply & api.im.v1.BatchPushReceiptToUsersReply.$Shape} BatchPushReceiptToUsersReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.BatchPushReceiptToUsersReply & api.im.v1.BatchPushReceiptToUsersReply.$Shape;

                /**
                 * Decodes a BatchPushReceiptToUsersReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.BatchPushReceiptToUsersReply & api.im.v1.BatchPushReceiptToUsersReply.$Shape} BatchPushReceiptToUsersReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.BatchPushReceiptToUsersReply & api.im.v1.BatchPushReceiptToUsersReply.$Shape;

                /**
                 * Verifies a BatchPushReceiptToUsersReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a BatchPushReceiptToUsersReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns BatchPushReceiptToUsersReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.BatchPushReceiptToUsersReply;

                /**
                 * Creates a plain object from a BatchPushReceiptToUsersReply message. Also converts values to other types if specified.
                 * @param message BatchPushReceiptToUsersReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.BatchPushReceiptToUsersReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this BatchPushReceiptToUsersReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for BatchPushReceiptToUsersReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace BatchPushReceiptToUsersReply {

                /** Properties of a BatchPushReceiptToUsersReply. */
                interface $Properties {

                    /** BatchPushReceiptToUsersReply totalDelivered */
                    totalDelivered?: (number|null);

                    /** BatchPushReceiptToUsersReply failedUserIds */
                    failedUserIds?: ((number|Long)[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a BatchPushReceiptToUsersReply. */
                type $Shape = api.im.v1.BatchPushReceiptToUsersReply.$Properties;
            }

            /**
             * Properties of a ReceiptBatchItem.
             * @deprecated Use api.im.v1.ReceiptBatchItem.$Properties instead.
             */
            interface IReceiptBatchItem extends api.im.v1.ReceiptBatchItem.$Properties {
            }

            /** Represents a ReceiptBatchItem. */
            class ReceiptBatchItem {

                /**
                 * Constructs a new ReceiptBatchItem.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.ReceiptBatchItem.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** ReceiptBatchItem userId. */
                userId: (number|Long);

                /** ReceiptBatchItem receipt. */
                receipt?: (api.im.v1.SendReceipt.$Properties|null);

                /**
                 * Creates a new ReceiptBatchItem instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns ReceiptBatchItem instance
                 */
                static create(properties: api.im.v1.ReceiptBatchItem.$Shape): api.im.v1.ReceiptBatchItem & api.im.v1.ReceiptBatchItem.$Shape;
                static create(properties?: api.im.v1.ReceiptBatchItem.$Properties): api.im.v1.ReceiptBatchItem;

                /**
                 * Encodes the specified ReceiptBatchItem message. Does not implicitly {@link api.im.v1.ReceiptBatchItem.verify|verify} messages.
                 * @param message ReceiptBatchItem message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.ReceiptBatchItem.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified ReceiptBatchItem message, length delimited. Does not implicitly {@link api.im.v1.ReceiptBatchItem.verify|verify} messages.
                 * @param message ReceiptBatchItem message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.ReceiptBatchItem.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a ReceiptBatchItem message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.ReceiptBatchItem & api.im.v1.ReceiptBatchItem.$Shape} ReceiptBatchItem
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.ReceiptBatchItem & api.im.v1.ReceiptBatchItem.$Shape;

                /**
                 * Decodes a ReceiptBatchItem message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.ReceiptBatchItem & api.im.v1.ReceiptBatchItem.$Shape} ReceiptBatchItem
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.ReceiptBatchItem & api.im.v1.ReceiptBatchItem.$Shape;

                /**
                 * Verifies a ReceiptBatchItem message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a ReceiptBatchItem message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns ReceiptBatchItem
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.ReceiptBatchItem;

                /**
                 * Creates a plain object from a ReceiptBatchItem message. Also converts values to other types if specified.
                 * @param message ReceiptBatchItem
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.ReceiptBatchItem, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this ReceiptBatchItem to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for ReceiptBatchItem
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace ReceiptBatchItem {

                /** Properties of a ReceiptBatchItem. */
                interface $Properties {

                    /** ReceiptBatchItem userId */
                    userId?: (number|Long|null);

                    /** ReceiptBatchItem receipt */
                    receipt?: (api.im.v1.SendReceipt.$Properties|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a ReceiptBatchItem. */
                type $Shape = api.im.v1.ReceiptBatchItem.$Properties;
            }

            /**
             * Properties of a BatchPushReceiptsToUsersRequest.
             * @deprecated Use api.im.v1.BatchPushReceiptsToUsersRequest.$Properties instead.
             */
            interface IBatchPushReceiptsToUsersRequest extends api.im.v1.BatchPushReceiptsToUsersRequest.$Properties {
            }

            /** Represents a BatchPushReceiptsToUsersRequest. */
            class BatchPushReceiptsToUsersRequest {

                /**
                 * Constructs a new BatchPushReceiptsToUsersRequest.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.BatchPushReceiptsToUsersRequest.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** BatchPushReceiptsToUsersRequest items. */
                items: api.im.v1.ReceiptBatchItem.$Properties[];

                /**
                 * Creates a new BatchPushReceiptsToUsersRequest instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns BatchPushReceiptsToUsersRequest instance
                 */
                static create(properties: api.im.v1.BatchPushReceiptsToUsersRequest.$Shape): api.im.v1.BatchPushReceiptsToUsersRequest & api.im.v1.BatchPushReceiptsToUsersRequest.$Shape;
                static create(properties?: api.im.v1.BatchPushReceiptsToUsersRequest.$Properties): api.im.v1.BatchPushReceiptsToUsersRequest;

                /**
                 * Encodes the specified BatchPushReceiptsToUsersRequest message. Does not implicitly {@link api.im.v1.BatchPushReceiptsToUsersRequest.verify|verify} messages.
                 * @param message BatchPushReceiptsToUsersRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.BatchPushReceiptsToUsersRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified BatchPushReceiptsToUsersRequest message, length delimited. Does not implicitly {@link api.im.v1.BatchPushReceiptsToUsersRequest.verify|verify} messages.
                 * @param message BatchPushReceiptsToUsersRequest message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.BatchPushReceiptsToUsersRequest.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a BatchPushReceiptsToUsersRequest message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.BatchPushReceiptsToUsersRequest & api.im.v1.BatchPushReceiptsToUsersRequest.$Shape} BatchPushReceiptsToUsersRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.BatchPushReceiptsToUsersRequest & api.im.v1.BatchPushReceiptsToUsersRequest.$Shape;

                /**
                 * Decodes a BatchPushReceiptsToUsersRequest message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.BatchPushReceiptsToUsersRequest & api.im.v1.BatchPushReceiptsToUsersRequest.$Shape} BatchPushReceiptsToUsersRequest
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.BatchPushReceiptsToUsersRequest & api.im.v1.BatchPushReceiptsToUsersRequest.$Shape;

                /**
                 * Verifies a BatchPushReceiptsToUsersRequest message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a BatchPushReceiptsToUsersRequest message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns BatchPushReceiptsToUsersRequest
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.BatchPushReceiptsToUsersRequest;

                /**
                 * Creates a plain object from a BatchPushReceiptsToUsersRequest message. Also converts values to other types if specified.
                 * @param message BatchPushReceiptsToUsersRequest
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.BatchPushReceiptsToUsersRequest, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this BatchPushReceiptsToUsersRequest to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for BatchPushReceiptsToUsersRequest
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace BatchPushReceiptsToUsersRequest {

                /** Properties of a BatchPushReceiptsToUsersRequest. */
                interface $Properties {

                    /** BatchPushReceiptsToUsersRequest items */
                    items?: (api.im.v1.ReceiptBatchItem.$Properties[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a BatchPushReceiptsToUsersRequest. */
                type $Shape = api.im.v1.BatchPushReceiptsToUsersRequest.$Properties;
            }

            /**
             * Properties of a BatchPushReceiptsToUsersReply.
             * @deprecated Use api.im.v1.BatchPushReceiptsToUsersReply.$Properties instead.
             */
            interface IBatchPushReceiptsToUsersReply extends api.im.v1.BatchPushReceiptsToUsersReply.$Properties {
            }

            /** Represents a BatchPushReceiptsToUsersReply. */
            class BatchPushReceiptsToUsersReply {

                /**
                 * Constructs a new BatchPushReceiptsToUsersReply.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: api.im.v1.BatchPushReceiptsToUsersReply.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** BatchPushReceiptsToUsersReply totalDelivered. */
                totalDelivered: number;

                /** BatchPushReceiptsToUsersReply failedItems. */
                failedItems: api.im.v1.ReceiptBatchItem.$Properties[];

                /**
                 * Creates a new BatchPushReceiptsToUsersReply instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns BatchPushReceiptsToUsersReply instance
                 */
                static create(properties: api.im.v1.BatchPushReceiptsToUsersReply.$Shape): api.im.v1.BatchPushReceiptsToUsersReply & api.im.v1.BatchPushReceiptsToUsersReply.$Shape;
                static create(properties?: api.im.v1.BatchPushReceiptsToUsersReply.$Properties): api.im.v1.BatchPushReceiptsToUsersReply;

                /**
                 * Encodes the specified BatchPushReceiptsToUsersReply message. Does not implicitly {@link api.im.v1.BatchPushReceiptsToUsersReply.verify|verify} messages.
                 * @param message BatchPushReceiptsToUsersReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: api.im.v1.BatchPushReceiptsToUsersReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified BatchPushReceiptsToUsersReply message, length delimited. Does not implicitly {@link api.im.v1.BatchPushReceiptsToUsersReply.verify|verify} messages.
                 * @param message BatchPushReceiptsToUsersReply message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: api.im.v1.BatchPushReceiptsToUsersReply.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a BatchPushReceiptsToUsersReply message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {api.im.v1.BatchPushReceiptsToUsersReply & api.im.v1.BatchPushReceiptsToUsersReply.$Shape} BatchPushReceiptsToUsersReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): api.im.v1.BatchPushReceiptsToUsersReply & api.im.v1.BatchPushReceiptsToUsersReply.$Shape;

                /**
                 * Decodes a BatchPushReceiptsToUsersReply message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {api.im.v1.BatchPushReceiptsToUsersReply & api.im.v1.BatchPushReceiptsToUsersReply.$Shape} BatchPushReceiptsToUsersReply
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): api.im.v1.BatchPushReceiptsToUsersReply & api.im.v1.BatchPushReceiptsToUsersReply.$Shape;

                /**
                 * Verifies a BatchPushReceiptsToUsersReply message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a BatchPushReceiptsToUsersReply message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns BatchPushReceiptsToUsersReply
                 */
                static fromObject(object: { [k: string]: any }): api.im.v1.BatchPushReceiptsToUsersReply;

                /**
                 * Creates a plain object from a BatchPushReceiptsToUsersReply message. Also converts values to other types if specified.
                 * @param message BatchPushReceiptsToUsersReply
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: api.im.v1.BatchPushReceiptsToUsersReply, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this BatchPushReceiptsToUsersReply to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for BatchPushReceiptsToUsersReply
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace BatchPushReceiptsToUsersReply {

                /** Properties of a BatchPushReceiptsToUsersReply. */
                interface $Properties {

                    /** BatchPushReceiptsToUsersReply totalDelivered */
                    totalDelivered?: (number|null);

                    /** BatchPushReceiptsToUsersReply failedItems */
                    failedItems?: (api.im.v1.ReceiptBatchItem.$Properties[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a BatchPushReceiptsToUsersReply. */
                type $Shape = api.im.v1.BatchPushReceiptsToUsersReply.$Properties;
            }
        }
    }
}

/** Namespace google. */
export namespace google {

    /** Namespace api. */
    namespace api {

        /**
         * Properties of a Http.
         * @deprecated Use google.api.Http.$Properties instead.
         */
        interface IHttp extends google.api.Http.$Properties {
        }

        /** Represents a Http. */
        class Http {

            /**
             * Constructs a new Http.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.api.Http.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** Http rules. */
            rules: google.api.HttpRule.$Properties[];

            /** Http fullyDecodeReservedExpansion. */
            fullyDecodeReservedExpansion: boolean;

            /**
             * Creates a new Http instance using the specified properties.
             * @param [properties] Properties to set
             * @returns Http instance
             */
            static create(properties: google.api.Http.$Shape): google.api.Http & google.api.Http.$Shape;
            static create(properties?: google.api.Http.$Properties): google.api.Http;

            /**
             * Encodes the specified Http message. Does not implicitly {@link google.api.Http.verify|verify} messages.
             * @param message Http message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.api.Http.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified Http message, length delimited. Does not implicitly {@link google.api.Http.verify|verify} messages.
             * @param message Http message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.api.Http.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a Http message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.api.Http & google.api.Http.$Shape} Http
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.api.Http & google.api.Http.$Shape;

            /**
             * Decodes a Http message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.api.Http & google.api.Http.$Shape} Http
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.api.Http & google.api.Http.$Shape;

            /**
             * Verifies a Http message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a Http message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns Http
             */
            static fromObject(object: { [k: string]: any }): google.api.Http;

            /**
             * Creates a plain object from a Http message. Also converts values to other types if specified.
             * @param message Http
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.api.Http, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this Http to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for Http
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace Http {

            /** Properties of a Http. */
            interface $Properties {

                /** Http rules */
                rules?: (google.api.HttpRule.$Properties[]|null);

                /** Http fullyDecodeReservedExpansion */
                fullyDecodeReservedExpansion?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a Http. */
            type $Shape = {
              rules?: google.api.HttpRule.$Shape[]|null;
              fullyDecodeReservedExpansion?: boolean|null;
              $unknowns?: Uint8Array[];
            };
        }

        /**
         * Properties of a HttpRule.
         * @deprecated Use google.api.HttpRule.$Properties instead.
         */
        interface IHttpRule extends google.api.HttpRule.$Properties {
        }

        /** Represents a HttpRule. */
        class HttpRule {

            /**
             * Constructs a new HttpRule.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.api.HttpRule.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** HttpRule selector. */
            selector: string;

            /** HttpRule get. */
            get?: (string|null);

            /** HttpRule put. */
            put?: (string|null);

            /** HttpRule post. */
            post?: (string|null);

            /** HttpRule delete. */
            delete?: (string|null);

            /** HttpRule patch. */
            patch?: (string|null);

            /** HttpRule custom. */
            custom?: (google.api.CustomHttpPattern.$Properties|null);

            /** HttpRule body. */
            body: string;

            /** HttpRule responseBody. */
            responseBody: string;

            /** HttpRule additionalBindings. */
            additionalBindings: google.api.HttpRule.$Properties[];

            /** HttpRule pattern. */
            pattern?: ("get"|"put"|"post"|"delete"|"patch"|"custom");

            /**
             * Creates a new HttpRule instance using the specified properties.
             * @param [properties] Properties to set
             * @returns HttpRule instance
             */
            static create(properties: google.api.HttpRule.$Shape): google.api.HttpRule & google.api.HttpRule.$Shape;
            static create(properties?: google.api.HttpRule.$Properties): google.api.HttpRule;

            /**
             * Encodes the specified HttpRule message. Does not implicitly {@link google.api.HttpRule.verify|verify} messages.
             * @param message HttpRule message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.api.HttpRule.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified HttpRule message, length delimited. Does not implicitly {@link google.api.HttpRule.verify|verify} messages.
             * @param message HttpRule message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.api.HttpRule.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a HttpRule message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.api.HttpRule & google.api.HttpRule.$Shape} HttpRule
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.api.HttpRule & google.api.HttpRule.$Shape;

            /**
             * Decodes a HttpRule message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.api.HttpRule & google.api.HttpRule.$Shape} HttpRule
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.api.HttpRule & google.api.HttpRule.$Shape;

            /**
             * Verifies a HttpRule message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a HttpRule message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns HttpRule
             */
            static fromObject(object: { [k: string]: any }): google.api.HttpRule;

            /**
             * Creates a plain object from a HttpRule message. Also converts values to other types if specified.
             * @param message HttpRule
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.api.HttpRule, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this HttpRule to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for HttpRule
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace HttpRule {

            /** Properties of a HttpRule. */
            interface $Properties {

                /** HttpRule selector */
                selector?: (string|null);

                /** HttpRule get */
                get?: (string|null);

                /** HttpRule put */
                put?: (string|null);

                /** HttpRule post */
                post?: (string|null);

                /** HttpRule delete */
                "delete"?: (string|null);

                /** HttpRule patch */
                patch?: (string|null);

                /** HttpRule custom */
                custom?: (google.api.CustomHttpPattern.$Properties|null);

                /** HttpRule body */
                body?: (string|null);

                /** HttpRule responseBody */
                responseBody?: (string|null);

                /** HttpRule additionalBindings */
                additionalBindings?: (google.api.HttpRule.$Properties[]|null);

                /** HttpRule pattern */
                pattern?: ("get"|"put"|"post"|"delete"|"patch"|"custom");

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Narrowed shape of a HttpRule. */
            type $Shape = {
              selector?: string|null;
              get?: string|null;
              put?: string|null;
              post?: string|null;
              "delete"?: string|null;
              patch?: string|null;
              custom?: google.api.CustomHttpPattern.$Shape|null;
              body?: string|null;
              responseBody?: string|null;
              additionalBindings?: google.api.HttpRule.$Shape[]|null;
              $unknowns?: Uint8Array[];
            } & (
              ({ pattern?: undefined; get?: null; put?: null; post?: null; "delete"?: null; patch?: null; custom?: null }|{ pattern?: "get"; get: string; put?: null; post?: null; "delete"?: null; patch?: null; custom?: null }|{ pattern?: "put"; get?: null; put: string; post?: null; "delete"?: null; patch?: null; custom?: null }|{ pattern?: "post"; get?: null; put?: null; post: string; "delete"?: null; patch?: null; custom?: null }|{ pattern?: "delete"; get?: null; put?: null; post?: null; "delete": string; patch?: null; custom?: null }|{ pattern?: "patch"; get?: null; put?: null; post?: null; "delete"?: null; patch: string; custom?: null }|{ pattern?: "custom"; get?: null; put?: null; post?: null; "delete"?: null; patch?: null; custom: google.api.CustomHttpPattern.$Shape })
            );
        }

        /**
         * Properties of a CustomHttpPattern.
         * @deprecated Use google.api.CustomHttpPattern.$Properties instead.
         */
        interface ICustomHttpPattern extends google.api.CustomHttpPattern.$Properties {
        }

        /** Represents a CustomHttpPattern. */
        class CustomHttpPattern {

            /**
             * Constructs a new CustomHttpPattern.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.api.CustomHttpPattern.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** CustomHttpPattern kind. */
            kind: string;

            /** CustomHttpPattern path. */
            path: string;

            /**
             * Creates a new CustomHttpPattern instance using the specified properties.
             * @param [properties] Properties to set
             * @returns CustomHttpPattern instance
             */
            static create(properties: google.api.CustomHttpPattern.$Shape): google.api.CustomHttpPattern & google.api.CustomHttpPattern.$Shape;
            static create(properties?: google.api.CustomHttpPattern.$Properties): google.api.CustomHttpPattern;

            /**
             * Encodes the specified CustomHttpPattern message. Does not implicitly {@link google.api.CustomHttpPattern.verify|verify} messages.
             * @param message CustomHttpPattern message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.api.CustomHttpPattern.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified CustomHttpPattern message, length delimited. Does not implicitly {@link google.api.CustomHttpPattern.verify|verify} messages.
             * @param message CustomHttpPattern message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.api.CustomHttpPattern.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a CustomHttpPattern message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.api.CustomHttpPattern & google.api.CustomHttpPattern.$Shape} CustomHttpPattern
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.api.CustomHttpPattern & google.api.CustomHttpPattern.$Shape;

            /**
             * Decodes a CustomHttpPattern message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.api.CustomHttpPattern & google.api.CustomHttpPattern.$Shape} CustomHttpPattern
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.api.CustomHttpPattern & google.api.CustomHttpPattern.$Shape;

            /**
             * Verifies a CustomHttpPattern message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a CustomHttpPattern message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns CustomHttpPattern
             */
            static fromObject(object: { [k: string]: any }): google.api.CustomHttpPattern;

            /**
             * Creates a plain object from a CustomHttpPattern message. Also converts values to other types if specified.
             * @param message CustomHttpPattern
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.api.CustomHttpPattern, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this CustomHttpPattern to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for CustomHttpPattern
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace CustomHttpPattern {

            /** Properties of a CustomHttpPattern. */
            interface $Properties {

                /** CustomHttpPattern kind */
                kind?: (string|null);

                /** CustomHttpPattern path */
                path?: (string|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a CustomHttpPattern. */
            type $Shape = google.api.CustomHttpPattern.$Properties;
        }
    }

    /** Namespace protobuf. */
    namespace protobuf {

        /**
         * Properties of a FileDescriptorSet.
         * @deprecated Use google.protobuf.FileDescriptorSet.$Properties instead.
         */
        interface IFileDescriptorSet extends google.protobuf.FileDescriptorSet.$Properties {
        }

        /** Represents a FileDescriptorSet. */
        class FileDescriptorSet {

            /**
             * Constructs a new FileDescriptorSet.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.FileDescriptorSet.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** FileDescriptorSet file. */
            file: google.protobuf.FileDescriptorProto.$Properties[];

            /**
             * Creates a new FileDescriptorSet instance using the specified properties.
             * @param [properties] Properties to set
             * @returns FileDescriptorSet instance
             */
            static create(properties: google.protobuf.FileDescriptorSet.$Shape): google.protobuf.FileDescriptorSet & google.protobuf.FileDescriptorSet.$Shape;
            static create(properties?: google.protobuf.FileDescriptorSet.$Properties): google.protobuf.FileDescriptorSet;

            /**
             * Encodes the specified FileDescriptorSet message. Does not implicitly {@link google.protobuf.FileDescriptorSet.verify|verify} messages.
             * @param message FileDescriptorSet message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.FileDescriptorSet.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified FileDescriptorSet message, length delimited. Does not implicitly {@link google.protobuf.FileDescriptorSet.verify|verify} messages.
             * @param message FileDescriptorSet message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.FileDescriptorSet.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a FileDescriptorSet message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.FileDescriptorSet & google.protobuf.FileDescriptorSet.$Shape} FileDescriptorSet
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.FileDescriptorSet & google.protobuf.FileDescriptorSet.$Shape;

            /**
             * Decodes a FileDescriptorSet message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.FileDescriptorSet & google.protobuf.FileDescriptorSet.$Shape} FileDescriptorSet
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.FileDescriptorSet & google.protobuf.FileDescriptorSet.$Shape;

            /**
             * Verifies a FileDescriptorSet message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a FileDescriptorSet message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns FileDescriptorSet
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.FileDescriptorSet;

            /**
             * Creates a plain object from a FileDescriptorSet message. Also converts values to other types if specified.
             * @param message FileDescriptorSet
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.FileDescriptorSet, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this FileDescriptorSet to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for FileDescriptorSet
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace FileDescriptorSet {

            /** Properties of a FileDescriptorSet. */
            interface $Properties {

                /** FileDescriptorSet file */
                file?: (google.protobuf.FileDescriptorProto.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a FileDescriptorSet. */
            type $Shape = {
              file?: google.protobuf.FileDescriptorProto.$Shape[]|null;
              $unknowns?: Uint8Array[];
            };
        }

        /**
         * Properties of a FileDescriptorProto.
         * @deprecated Use google.protobuf.FileDescriptorProto.$Properties instead.
         */
        interface IFileDescriptorProto extends google.protobuf.FileDescriptorProto.$Properties {
        }

        /** Represents a FileDescriptorProto. */
        class FileDescriptorProto {

            /**
             * Constructs a new FileDescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.FileDescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** FileDescriptorProto name. */
            name: string;

            /** FileDescriptorProto package. */
            package: string;

            /** FileDescriptorProto dependency. */
            dependency: string[];

            /** FileDescriptorProto publicDependency. */
            publicDependency: number[];

            /** FileDescriptorProto weakDependency. */
            weakDependency: number[];

            /** FileDescriptorProto messageType. */
            messageType: google.protobuf.DescriptorProto.$Properties[];

            /** FileDescriptorProto enumType. */
            enumType: google.protobuf.EnumDescriptorProto.$Properties[];

            /** FileDescriptorProto service. */
            service: google.protobuf.ServiceDescriptorProto.$Properties[];

            /** FileDescriptorProto extension. */
            extension: google.protobuf.FieldDescriptorProto.$Properties[];

            /** FileDescriptorProto options. */
            options?: (google.protobuf.FileOptions.$Properties|null);

            /** FileDescriptorProto sourceCodeInfo. */
            sourceCodeInfo?: (google.protobuf.SourceCodeInfo.$Properties|null);

            /** FileDescriptorProto syntax. */
            syntax: string;

            /**
             * Creates a new FileDescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns FileDescriptorProto instance
             */
            static create(properties: google.protobuf.FileDescriptorProto.$Shape): google.protobuf.FileDescriptorProto & google.protobuf.FileDescriptorProto.$Shape;
            static create(properties?: google.protobuf.FileDescriptorProto.$Properties): google.protobuf.FileDescriptorProto;

            /**
             * Encodes the specified FileDescriptorProto message. Does not implicitly {@link google.protobuf.FileDescriptorProto.verify|verify} messages.
             * @param message FileDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.FileDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified FileDescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.FileDescriptorProto.verify|verify} messages.
             * @param message FileDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.FileDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a FileDescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.FileDescriptorProto & google.protobuf.FileDescriptorProto.$Shape} FileDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.FileDescriptorProto & google.protobuf.FileDescriptorProto.$Shape;

            /**
             * Decodes a FileDescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.FileDescriptorProto & google.protobuf.FileDescriptorProto.$Shape} FileDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.FileDescriptorProto & google.protobuf.FileDescriptorProto.$Shape;

            /**
             * Verifies a FileDescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a FileDescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns FileDescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.FileDescriptorProto;

            /**
             * Creates a plain object from a FileDescriptorProto message. Also converts values to other types if specified.
             * @param message FileDescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.FileDescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this FileDescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for FileDescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace FileDescriptorProto {

            /** Properties of a FileDescriptorProto. */
            interface $Properties {

                /** FileDescriptorProto name */
                name?: (string|null);

                /** FileDescriptorProto package */
                "package"?: (string|null);

                /** FileDescriptorProto dependency */
                dependency?: (string[]|null);

                /** FileDescriptorProto publicDependency */
                publicDependency?: (number[]|null);

                /** FileDescriptorProto weakDependency */
                weakDependency?: (number[]|null);

                /** FileDescriptorProto messageType */
                messageType?: (google.protobuf.DescriptorProto.$Properties[]|null);

                /** FileDescriptorProto enumType */
                enumType?: (google.protobuf.EnumDescriptorProto.$Properties[]|null);

                /** FileDescriptorProto service */
                service?: (google.protobuf.ServiceDescriptorProto.$Properties[]|null);

                /** FileDescriptorProto extension */
                extension?: (google.protobuf.FieldDescriptorProto.$Properties[]|null);

                /** FileDescriptorProto options */
                options?: (google.protobuf.FileOptions.$Properties|null);

                /** FileDescriptorProto sourceCodeInfo */
                sourceCodeInfo?: (google.protobuf.SourceCodeInfo.$Properties|null);

                /** FileDescriptorProto syntax */
                syntax?: (string|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a FileDescriptorProto. */
            type $Shape = {
              name?: string|null;
              "package"?: string|null;
              dependency?: string[]|null;
              publicDependency?: number[]|null;
              weakDependency?: number[]|null;
              messageType?: google.protobuf.DescriptorProto.$Shape[]|null;
              enumType?: google.protobuf.EnumDescriptorProto.$Shape[]|null;
              service?: google.protobuf.ServiceDescriptorProto.$Shape[]|null;
              extension?: google.protobuf.FieldDescriptorProto.$Shape[]|null;
              options?: google.protobuf.FileOptions.$Shape|null;
              sourceCodeInfo?: google.protobuf.SourceCodeInfo.$Shape|null;
              syntax?: string|null;
              $unknowns?: Uint8Array[];
            };
        }

        /**
         * Properties of a DescriptorProto.
         * @deprecated Use google.protobuf.DescriptorProto.$Properties instead.
         */
        interface IDescriptorProto extends google.protobuf.DescriptorProto.$Properties {
        }

        /** Represents a DescriptorProto. */
        class DescriptorProto {

            /**
             * Constructs a new DescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.DescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** DescriptorProto name. */
            name: string;

            /** DescriptorProto field. */
            field: google.protobuf.FieldDescriptorProto.$Properties[];

            /** DescriptorProto extension. */
            extension: google.protobuf.FieldDescriptorProto.$Properties[];

            /** DescriptorProto nestedType. */
            nestedType: google.protobuf.DescriptorProto.$Properties[];

            /** DescriptorProto enumType. */
            enumType: google.protobuf.EnumDescriptorProto.$Properties[];

            /** DescriptorProto extensionRange. */
            extensionRange: google.protobuf.DescriptorProto.ExtensionRange.$Properties[];

            /** DescriptorProto oneofDecl. */
            oneofDecl: google.protobuf.OneofDescriptorProto.$Properties[];

            /** DescriptorProto options. */
            options?: (google.protobuf.MessageOptions.$Properties|null);

            /** DescriptorProto reservedRange. */
            reservedRange: google.protobuf.DescriptorProto.ReservedRange.$Properties[];

            /** DescriptorProto reservedName. */
            reservedName: string[];

            /**
             * Creates a new DescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns DescriptorProto instance
             */
            static create(properties: google.protobuf.DescriptorProto.$Shape): google.protobuf.DescriptorProto & google.protobuf.DescriptorProto.$Shape;
            static create(properties?: google.protobuf.DescriptorProto.$Properties): google.protobuf.DescriptorProto;

            /**
             * Encodes the specified DescriptorProto message. Does not implicitly {@link google.protobuf.DescriptorProto.verify|verify} messages.
             * @param message DescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.DescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified DescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.DescriptorProto.verify|verify} messages.
             * @param message DescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.DescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a DescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.DescriptorProto & google.protobuf.DescriptorProto.$Shape} DescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.DescriptorProto & google.protobuf.DescriptorProto.$Shape;

            /**
             * Decodes a DescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.DescriptorProto & google.protobuf.DescriptorProto.$Shape} DescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.DescriptorProto & google.protobuf.DescriptorProto.$Shape;

            /**
             * Verifies a DescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a DescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns DescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.DescriptorProto;

            /**
             * Creates a plain object from a DescriptorProto message. Also converts values to other types if specified.
             * @param message DescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.DescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this DescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for DescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace DescriptorProto {

            /** Properties of a DescriptorProto. */
            interface $Properties {

                /** DescriptorProto name */
                name?: (string|null);

                /** DescriptorProto field */
                field?: (google.protobuf.FieldDescriptorProto.$Properties[]|null);

                /** DescriptorProto extension */
                extension?: (google.protobuf.FieldDescriptorProto.$Properties[]|null);

                /** DescriptorProto nestedType */
                nestedType?: (google.protobuf.DescriptorProto.$Properties[]|null);

                /** DescriptorProto enumType */
                enumType?: (google.protobuf.EnumDescriptorProto.$Properties[]|null);

                /** DescriptorProto extensionRange */
                extensionRange?: (google.protobuf.DescriptorProto.ExtensionRange.$Properties[]|null);

                /** DescriptorProto oneofDecl */
                oneofDecl?: (google.protobuf.OneofDescriptorProto.$Properties[]|null);

                /** DescriptorProto options */
                options?: (google.protobuf.MessageOptions.$Properties|null);

                /** DescriptorProto reservedRange */
                reservedRange?: (google.protobuf.DescriptorProto.ReservedRange.$Properties[]|null);

                /** DescriptorProto reservedName */
                reservedName?: (string[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a DescriptorProto. */
            type $Shape = google.protobuf.DescriptorProto.$Properties;

            /**
             * Properties of an ExtensionRange.
             * @deprecated Use google.protobuf.DescriptorProto.ExtensionRange.$Properties instead.
             */
            interface IExtensionRange extends google.protobuf.DescriptorProto.ExtensionRange.$Properties {
            }

            /** Represents an ExtensionRange. */
            class ExtensionRange {

                /**
                 * Constructs a new ExtensionRange.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: google.protobuf.DescriptorProto.ExtensionRange.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** ExtensionRange start. */
                start: number;

                /** ExtensionRange end. */
                end: number;

                /** ExtensionRange options. */
                options?: (google.protobuf.ExtensionRangeOptions.$Properties|null);

                /**
                 * Creates a new ExtensionRange instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns ExtensionRange instance
                 */
                static create(properties: google.protobuf.DescriptorProto.ExtensionRange.$Shape): google.protobuf.DescriptorProto.ExtensionRange & google.protobuf.DescriptorProto.ExtensionRange.$Shape;
                static create(properties?: google.protobuf.DescriptorProto.ExtensionRange.$Properties): google.protobuf.DescriptorProto.ExtensionRange;

                /**
                 * Encodes the specified ExtensionRange message. Does not implicitly {@link google.protobuf.DescriptorProto.ExtensionRange.verify|verify} messages.
                 * @param message ExtensionRange message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: google.protobuf.DescriptorProto.ExtensionRange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified ExtensionRange message, length delimited. Does not implicitly {@link google.protobuf.DescriptorProto.ExtensionRange.verify|verify} messages.
                 * @param message ExtensionRange message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: google.protobuf.DescriptorProto.ExtensionRange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an ExtensionRange message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {google.protobuf.DescriptorProto.ExtensionRange & google.protobuf.DescriptorProto.ExtensionRange.$Shape} ExtensionRange
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.DescriptorProto.ExtensionRange & google.protobuf.DescriptorProto.ExtensionRange.$Shape;

                /**
                 * Decodes an ExtensionRange message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {google.protobuf.DescriptorProto.ExtensionRange & google.protobuf.DescriptorProto.ExtensionRange.$Shape} ExtensionRange
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.DescriptorProto.ExtensionRange & google.protobuf.DescriptorProto.ExtensionRange.$Shape;

                /**
                 * Verifies an ExtensionRange message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an ExtensionRange message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns ExtensionRange
                 */
                static fromObject(object: { [k: string]: any }): google.protobuf.DescriptorProto.ExtensionRange;

                /**
                 * Creates a plain object from an ExtensionRange message. Also converts values to other types if specified.
                 * @param message ExtensionRange
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: google.protobuf.DescriptorProto.ExtensionRange, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this ExtensionRange to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for ExtensionRange
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace ExtensionRange {

                /** Properties of an ExtensionRange. */
                interface $Properties {

                    /** ExtensionRange start */
                    start?: (number|null);

                    /** ExtensionRange end */
                    end?: (number|null);

                    /** ExtensionRange options */
                    options?: (google.protobuf.ExtensionRangeOptions.$Properties|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an ExtensionRange. */
                type $Shape = google.protobuf.DescriptorProto.ExtensionRange.$Properties;
            }

            /**
             * Properties of a ReservedRange.
             * @deprecated Use google.protobuf.DescriptorProto.ReservedRange.$Properties instead.
             */
            interface IReservedRange extends google.protobuf.DescriptorProto.ReservedRange.$Properties {
            }

            /** Represents a ReservedRange. */
            class ReservedRange {

                /**
                 * Constructs a new ReservedRange.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: google.protobuf.DescriptorProto.ReservedRange.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** ReservedRange start. */
                start: number;

                /** ReservedRange end. */
                end: number;

                /**
                 * Creates a new ReservedRange instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns ReservedRange instance
                 */
                static create(properties: google.protobuf.DescriptorProto.ReservedRange.$Shape): google.protobuf.DescriptorProto.ReservedRange & google.protobuf.DescriptorProto.ReservedRange.$Shape;
                static create(properties?: google.protobuf.DescriptorProto.ReservedRange.$Properties): google.protobuf.DescriptorProto.ReservedRange;

                /**
                 * Encodes the specified ReservedRange message. Does not implicitly {@link google.protobuf.DescriptorProto.ReservedRange.verify|verify} messages.
                 * @param message ReservedRange message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: google.protobuf.DescriptorProto.ReservedRange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified ReservedRange message, length delimited. Does not implicitly {@link google.protobuf.DescriptorProto.ReservedRange.verify|verify} messages.
                 * @param message ReservedRange message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: google.protobuf.DescriptorProto.ReservedRange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a ReservedRange message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {google.protobuf.DescriptorProto.ReservedRange & google.protobuf.DescriptorProto.ReservedRange.$Shape} ReservedRange
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.DescriptorProto.ReservedRange & google.protobuf.DescriptorProto.ReservedRange.$Shape;

                /**
                 * Decodes a ReservedRange message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {google.protobuf.DescriptorProto.ReservedRange & google.protobuf.DescriptorProto.ReservedRange.$Shape} ReservedRange
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.DescriptorProto.ReservedRange & google.protobuf.DescriptorProto.ReservedRange.$Shape;

                /**
                 * Verifies a ReservedRange message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a ReservedRange message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns ReservedRange
                 */
                static fromObject(object: { [k: string]: any }): google.protobuf.DescriptorProto.ReservedRange;

                /**
                 * Creates a plain object from a ReservedRange message. Also converts values to other types if specified.
                 * @param message ReservedRange
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: google.protobuf.DescriptorProto.ReservedRange, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this ReservedRange to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for ReservedRange
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace ReservedRange {

                /** Properties of a ReservedRange. */
                interface $Properties {

                    /** ReservedRange start */
                    start?: (number|null);

                    /** ReservedRange end */
                    end?: (number|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a ReservedRange. */
                type $Shape = google.protobuf.DescriptorProto.ReservedRange.$Properties;
            }
        }

        /**
         * Properties of an ExtensionRangeOptions.
         * @deprecated Use google.protobuf.ExtensionRangeOptions.$Properties instead.
         */
        interface IExtensionRangeOptions extends google.protobuf.ExtensionRangeOptions.$Properties {
        }

        /** Represents an ExtensionRangeOptions. */
        class ExtensionRangeOptions {

            /**
             * Constructs a new ExtensionRangeOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.ExtensionRangeOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ExtensionRangeOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new ExtensionRangeOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ExtensionRangeOptions instance
             */
            static create(properties: google.protobuf.ExtensionRangeOptions.$Shape): google.protobuf.ExtensionRangeOptions & google.protobuf.ExtensionRangeOptions.$Shape;
            static create(properties?: google.protobuf.ExtensionRangeOptions.$Properties): google.protobuf.ExtensionRangeOptions;

            /**
             * Encodes the specified ExtensionRangeOptions message. Does not implicitly {@link google.protobuf.ExtensionRangeOptions.verify|verify} messages.
             * @param message ExtensionRangeOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.ExtensionRangeOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ExtensionRangeOptions message, length delimited. Does not implicitly {@link google.protobuf.ExtensionRangeOptions.verify|verify} messages.
             * @param message ExtensionRangeOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.ExtensionRangeOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an ExtensionRangeOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.ExtensionRangeOptions & google.protobuf.ExtensionRangeOptions.$Shape} ExtensionRangeOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.ExtensionRangeOptions & google.protobuf.ExtensionRangeOptions.$Shape;

            /**
             * Decodes an ExtensionRangeOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.ExtensionRangeOptions & google.protobuf.ExtensionRangeOptions.$Shape} ExtensionRangeOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.ExtensionRangeOptions & google.protobuf.ExtensionRangeOptions.$Shape;

            /**
             * Verifies an ExtensionRangeOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates an ExtensionRangeOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns ExtensionRangeOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.ExtensionRangeOptions;

            /**
             * Creates a plain object from an ExtensionRangeOptions message. Also converts values to other types if specified.
             * @param message ExtensionRangeOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.ExtensionRangeOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this ExtensionRangeOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for ExtensionRangeOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ExtensionRangeOptions {

            /** Properties of an ExtensionRangeOptions. */
            interface $Properties {

                /** ExtensionRangeOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an ExtensionRangeOptions. */
            type $Shape = google.protobuf.ExtensionRangeOptions.$Properties;
        }

        /**
         * Properties of a FieldDescriptorProto.
         * @deprecated Use google.protobuf.FieldDescriptorProto.$Properties instead.
         */
        interface IFieldDescriptorProto extends google.protobuf.FieldDescriptorProto.$Properties {
        }

        /** Represents a FieldDescriptorProto. */
        class FieldDescriptorProto {

            /**
             * Constructs a new FieldDescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.FieldDescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** FieldDescriptorProto name. */
            name: string;

            /** FieldDescriptorProto number. */
            number: number;

            /** FieldDescriptorProto label. */
            label: google.protobuf.FieldDescriptorProto.Label;

            /** FieldDescriptorProto type. */
            type: google.protobuf.FieldDescriptorProto.Type;

            /** FieldDescriptorProto typeName. */
            typeName: string;

            /** FieldDescriptorProto extendee. */
            extendee: string;

            /** FieldDescriptorProto defaultValue. */
            defaultValue: string;

            /** FieldDescriptorProto oneofIndex. */
            oneofIndex: number;

            /** FieldDescriptorProto jsonName. */
            jsonName: string;

            /** FieldDescriptorProto options. */
            options?: (google.protobuf.FieldOptions.$Properties|null);

            /** FieldDescriptorProto proto3Optional. */
            proto3Optional: boolean;

            /**
             * Creates a new FieldDescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns FieldDescriptorProto instance
             */
            static create(properties: google.protobuf.FieldDescriptorProto.$Shape): google.protobuf.FieldDescriptorProto & google.protobuf.FieldDescriptorProto.$Shape;
            static create(properties?: google.protobuf.FieldDescriptorProto.$Properties): google.protobuf.FieldDescriptorProto;

            /**
             * Encodes the specified FieldDescriptorProto message. Does not implicitly {@link google.protobuf.FieldDescriptorProto.verify|verify} messages.
             * @param message FieldDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.FieldDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified FieldDescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.FieldDescriptorProto.verify|verify} messages.
             * @param message FieldDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.FieldDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a FieldDescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.FieldDescriptorProto & google.protobuf.FieldDescriptorProto.$Shape} FieldDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.FieldDescriptorProto & google.protobuf.FieldDescriptorProto.$Shape;

            /**
             * Decodes a FieldDescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.FieldDescriptorProto & google.protobuf.FieldDescriptorProto.$Shape} FieldDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.FieldDescriptorProto & google.protobuf.FieldDescriptorProto.$Shape;

            /**
             * Verifies a FieldDescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a FieldDescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns FieldDescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.FieldDescriptorProto;

            /**
             * Creates a plain object from a FieldDescriptorProto message. Also converts values to other types if specified.
             * @param message FieldDescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.FieldDescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this FieldDescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for FieldDescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace FieldDescriptorProto {

            /** Properties of a FieldDescriptorProto. */
            interface $Properties {

                /** FieldDescriptorProto name */
                name?: (string|null);

                /** FieldDescriptorProto number */
                number?: (number|null);

                /** FieldDescriptorProto label */
                label?: (google.protobuf.FieldDescriptorProto.Label|null);

                /** FieldDescriptorProto type */
                type?: (google.protobuf.FieldDescriptorProto.Type|null);

                /** FieldDescriptorProto typeName */
                typeName?: (string|null);

                /** FieldDescriptorProto extendee */
                extendee?: (string|null);

                /** FieldDescriptorProto defaultValue */
                defaultValue?: (string|null);

                /** FieldDescriptorProto oneofIndex */
                oneofIndex?: (number|null);

                /** FieldDescriptorProto jsonName */
                jsonName?: (string|null);

                /** FieldDescriptorProto options */
                options?: (google.protobuf.FieldOptions.$Properties|null);

                /** FieldDescriptorProto proto3Optional */
                proto3Optional?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a FieldDescriptorProto. */
            type $Shape = google.protobuf.FieldDescriptorProto.$Properties;

            /** Type enum. */
            enum Type {

                /** TYPE_DOUBLE value */
                TYPE_DOUBLE = 1,

                /** TYPE_FLOAT value */
                TYPE_FLOAT = 2,

                /** TYPE_INT64 value */
                TYPE_INT64 = 3,

                /** TYPE_UINT64 value */
                TYPE_UINT64 = 4,

                /** TYPE_INT32 value */
                TYPE_INT32 = 5,

                /** TYPE_FIXED64 value */
                TYPE_FIXED64 = 6,

                /** TYPE_FIXED32 value */
                TYPE_FIXED32 = 7,

                /** TYPE_BOOL value */
                TYPE_BOOL = 8,

                /** TYPE_STRING value */
                TYPE_STRING = 9,

                /** TYPE_GROUP value */
                TYPE_GROUP = 10,

                /** TYPE_MESSAGE value */
                TYPE_MESSAGE = 11,

                /** TYPE_BYTES value */
                TYPE_BYTES = 12,

                /** TYPE_UINT32 value */
                TYPE_UINT32 = 13,

                /** TYPE_ENUM value */
                TYPE_ENUM = 14,

                /** TYPE_SFIXED32 value */
                TYPE_SFIXED32 = 15,

                /** TYPE_SFIXED64 value */
                TYPE_SFIXED64 = 16,

                /** TYPE_SINT32 value */
                TYPE_SINT32 = 17,

                /** TYPE_SINT64 value */
                TYPE_SINT64 = 18
            }

            /** Label enum. */
            enum Label {

                /** LABEL_OPTIONAL value */
                LABEL_OPTIONAL = 1,

                /** LABEL_REQUIRED value */
                LABEL_REQUIRED = 2,

                /** LABEL_REPEATED value */
                LABEL_REPEATED = 3
            }
        }

        /**
         * Properties of a OneofDescriptorProto.
         * @deprecated Use google.protobuf.OneofDescriptorProto.$Properties instead.
         */
        interface IOneofDescriptorProto extends google.protobuf.OneofDescriptorProto.$Properties {
        }

        /** Represents a OneofDescriptorProto. */
        class OneofDescriptorProto {

            /**
             * Constructs a new OneofDescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.OneofDescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** OneofDescriptorProto name. */
            name: string;

            /** OneofDescriptorProto options. */
            options?: (google.protobuf.OneofOptions.$Properties|null);

            /**
             * Creates a new OneofDescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns OneofDescriptorProto instance
             */
            static create(properties: google.protobuf.OneofDescriptorProto.$Shape): google.protobuf.OneofDescriptorProto & google.protobuf.OneofDescriptorProto.$Shape;
            static create(properties?: google.protobuf.OneofDescriptorProto.$Properties): google.protobuf.OneofDescriptorProto;

            /**
             * Encodes the specified OneofDescriptorProto message. Does not implicitly {@link google.protobuf.OneofDescriptorProto.verify|verify} messages.
             * @param message OneofDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.OneofDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified OneofDescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.OneofDescriptorProto.verify|verify} messages.
             * @param message OneofDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.OneofDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a OneofDescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.OneofDescriptorProto & google.protobuf.OneofDescriptorProto.$Shape} OneofDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.OneofDescriptorProto & google.protobuf.OneofDescriptorProto.$Shape;

            /**
             * Decodes a OneofDescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.OneofDescriptorProto & google.protobuf.OneofDescriptorProto.$Shape} OneofDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.OneofDescriptorProto & google.protobuf.OneofDescriptorProto.$Shape;

            /**
             * Verifies a OneofDescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a OneofDescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns OneofDescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.OneofDescriptorProto;

            /**
             * Creates a plain object from a OneofDescriptorProto message. Also converts values to other types if specified.
             * @param message OneofDescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.OneofDescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this OneofDescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for OneofDescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace OneofDescriptorProto {

            /** Properties of a OneofDescriptorProto. */
            interface $Properties {

                /** OneofDescriptorProto name */
                name?: (string|null);

                /** OneofDescriptorProto options */
                options?: (google.protobuf.OneofOptions.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a OneofDescriptorProto. */
            type $Shape = google.protobuf.OneofDescriptorProto.$Properties;
        }

        /**
         * Properties of an EnumDescriptorProto.
         * @deprecated Use google.protobuf.EnumDescriptorProto.$Properties instead.
         */
        interface IEnumDescriptorProto extends google.protobuf.EnumDescriptorProto.$Properties {
        }

        /** Represents an EnumDescriptorProto. */
        class EnumDescriptorProto {

            /**
             * Constructs a new EnumDescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.EnumDescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EnumDescriptorProto name. */
            name: string;

            /** EnumDescriptorProto value. */
            value: google.protobuf.EnumValueDescriptorProto.$Properties[];

            /** EnumDescriptorProto options. */
            options?: (google.protobuf.EnumOptions.$Properties|null);

            /** EnumDescriptorProto reservedRange. */
            reservedRange: google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties[];

            /** EnumDescriptorProto reservedName. */
            reservedName: string[];

            /**
             * Creates a new EnumDescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EnumDescriptorProto instance
             */
            static create(properties: google.protobuf.EnumDescriptorProto.$Shape): google.protobuf.EnumDescriptorProto & google.protobuf.EnumDescriptorProto.$Shape;
            static create(properties?: google.protobuf.EnumDescriptorProto.$Properties): google.protobuf.EnumDescriptorProto;

            /**
             * Encodes the specified EnumDescriptorProto message. Does not implicitly {@link google.protobuf.EnumDescriptorProto.verify|verify} messages.
             * @param message EnumDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.EnumDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EnumDescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.EnumDescriptorProto.verify|verify} messages.
             * @param message EnumDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.EnumDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EnumDescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.EnumDescriptorProto & google.protobuf.EnumDescriptorProto.$Shape} EnumDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.EnumDescriptorProto & google.protobuf.EnumDescriptorProto.$Shape;

            /**
             * Decodes an EnumDescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.EnumDescriptorProto & google.protobuf.EnumDescriptorProto.$Shape} EnumDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.EnumDescriptorProto & google.protobuf.EnumDescriptorProto.$Shape;

            /**
             * Verifies an EnumDescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates an EnumDescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns EnumDescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.EnumDescriptorProto;

            /**
             * Creates a plain object from an EnumDescriptorProto message. Also converts values to other types if specified.
             * @param message EnumDescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.EnumDescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this EnumDescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for EnumDescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EnumDescriptorProto {

            /** Properties of an EnumDescriptorProto. */
            interface $Properties {

                /** EnumDescriptorProto name */
                name?: (string|null);

                /** EnumDescriptorProto value */
                value?: (google.protobuf.EnumValueDescriptorProto.$Properties[]|null);

                /** EnumDescriptorProto options */
                options?: (google.protobuf.EnumOptions.$Properties|null);

                /** EnumDescriptorProto reservedRange */
                reservedRange?: (google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties[]|null);

                /** EnumDescriptorProto reservedName */
                reservedName?: (string[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EnumDescriptorProto. */
            type $Shape = google.protobuf.EnumDescriptorProto.$Properties;

            /**
             * Properties of an EnumReservedRange.
             * @deprecated Use google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties instead.
             */
            interface IEnumReservedRange extends google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties {
            }

            /** Represents an EnumReservedRange. */
            class EnumReservedRange {

                /**
                 * Constructs a new EnumReservedRange.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** EnumReservedRange start. */
                start: number;

                /** EnumReservedRange end. */
                end: number;

                /**
                 * Creates a new EnumReservedRange instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns EnumReservedRange instance
                 */
                static create(properties: google.protobuf.EnumDescriptorProto.EnumReservedRange.$Shape): google.protobuf.EnumDescriptorProto.EnumReservedRange & google.protobuf.EnumDescriptorProto.EnumReservedRange.$Shape;
                static create(properties?: google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties): google.protobuf.EnumDescriptorProto.EnumReservedRange;

                /**
                 * Encodes the specified EnumReservedRange message. Does not implicitly {@link google.protobuf.EnumDescriptorProto.EnumReservedRange.verify|verify} messages.
                 * @param message EnumReservedRange message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified EnumReservedRange message, length delimited. Does not implicitly {@link google.protobuf.EnumDescriptorProto.EnumReservedRange.verify|verify} messages.
                 * @param message EnumReservedRange message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an EnumReservedRange message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {google.protobuf.EnumDescriptorProto.EnumReservedRange & google.protobuf.EnumDescriptorProto.EnumReservedRange.$Shape} EnumReservedRange
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.EnumDescriptorProto.EnumReservedRange & google.protobuf.EnumDescriptorProto.EnumReservedRange.$Shape;

                /**
                 * Decodes an EnumReservedRange message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {google.protobuf.EnumDescriptorProto.EnumReservedRange & google.protobuf.EnumDescriptorProto.EnumReservedRange.$Shape} EnumReservedRange
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.EnumDescriptorProto.EnumReservedRange & google.protobuf.EnumDescriptorProto.EnumReservedRange.$Shape;

                /**
                 * Verifies an EnumReservedRange message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an EnumReservedRange message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns EnumReservedRange
                 */
                static fromObject(object: { [k: string]: any }): google.protobuf.EnumDescriptorProto.EnumReservedRange;

                /**
                 * Creates a plain object from an EnumReservedRange message. Also converts values to other types if specified.
                 * @param message EnumReservedRange
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: google.protobuf.EnumDescriptorProto.EnumReservedRange, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this EnumReservedRange to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for EnumReservedRange
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace EnumReservedRange {

                /** Properties of an EnumReservedRange. */
                interface $Properties {

                    /** EnumReservedRange start */
                    start?: (number|null);

                    /** EnumReservedRange end */
                    end?: (number|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an EnumReservedRange. */
                type $Shape = google.protobuf.EnumDescriptorProto.EnumReservedRange.$Properties;
            }
        }

        /**
         * Properties of an EnumValueDescriptorProto.
         * @deprecated Use google.protobuf.EnumValueDescriptorProto.$Properties instead.
         */
        interface IEnumValueDescriptorProto extends google.protobuf.EnumValueDescriptorProto.$Properties {
        }

        /** Represents an EnumValueDescriptorProto. */
        class EnumValueDescriptorProto {

            /**
             * Constructs a new EnumValueDescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.EnumValueDescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EnumValueDescriptorProto name. */
            name: string;

            /** EnumValueDescriptorProto number. */
            number: number;

            /** EnumValueDescriptorProto options. */
            options?: (google.protobuf.EnumValueOptions.$Properties|null);

            /**
             * Creates a new EnumValueDescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EnumValueDescriptorProto instance
             */
            static create(properties: google.protobuf.EnumValueDescriptorProto.$Shape): google.protobuf.EnumValueDescriptorProto & google.protobuf.EnumValueDescriptorProto.$Shape;
            static create(properties?: google.protobuf.EnumValueDescriptorProto.$Properties): google.protobuf.EnumValueDescriptorProto;

            /**
             * Encodes the specified EnumValueDescriptorProto message. Does not implicitly {@link google.protobuf.EnumValueDescriptorProto.verify|verify} messages.
             * @param message EnumValueDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.EnumValueDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EnumValueDescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.EnumValueDescriptorProto.verify|verify} messages.
             * @param message EnumValueDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.EnumValueDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EnumValueDescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.EnumValueDescriptorProto & google.protobuf.EnumValueDescriptorProto.$Shape} EnumValueDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.EnumValueDescriptorProto & google.protobuf.EnumValueDescriptorProto.$Shape;

            /**
             * Decodes an EnumValueDescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.EnumValueDescriptorProto & google.protobuf.EnumValueDescriptorProto.$Shape} EnumValueDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.EnumValueDescriptorProto & google.protobuf.EnumValueDescriptorProto.$Shape;

            /**
             * Verifies an EnumValueDescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates an EnumValueDescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns EnumValueDescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.EnumValueDescriptorProto;

            /**
             * Creates a plain object from an EnumValueDescriptorProto message. Also converts values to other types if specified.
             * @param message EnumValueDescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.EnumValueDescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this EnumValueDescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for EnumValueDescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EnumValueDescriptorProto {

            /** Properties of an EnumValueDescriptorProto. */
            interface $Properties {

                /** EnumValueDescriptorProto name */
                name?: (string|null);

                /** EnumValueDescriptorProto number */
                number?: (number|null);

                /** EnumValueDescriptorProto options */
                options?: (google.protobuf.EnumValueOptions.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EnumValueDescriptorProto. */
            type $Shape = google.protobuf.EnumValueDescriptorProto.$Properties;
        }

        /**
         * Properties of a ServiceDescriptorProto.
         * @deprecated Use google.protobuf.ServiceDescriptorProto.$Properties instead.
         */
        interface IServiceDescriptorProto extends google.protobuf.ServiceDescriptorProto.$Properties {
        }

        /** Represents a ServiceDescriptorProto. */
        class ServiceDescriptorProto {

            /**
             * Constructs a new ServiceDescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.ServiceDescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ServiceDescriptorProto name. */
            name: string;

            /** ServiceDescriptorProto method. */
            method: google.protobuf.MethodDescriptorProto.$Properties[];

            /** ServiceDescriptorProto options. */
            options?: (google.protobuf.ServiceOptions.$Properties|null);

            /**
             * Creates a new ServiceDescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ServiceDescriptorProto instance
             */
            static create(properties: google.protobuf.ServiceDescriptorProto.$Shape): google.protobuf.ServiceDescriptorProto & google.protobuf.ServiceDescriptorProto.$Shape;
            static create(properties?: google.protobuf.ServiceDescriptorProto.$Properties): google.protobuf.ServiceDescriptorProto;

            /**
             * Encodes the specified ServiceDescriptorProto message. Does not implicitly {@link google.protobuf.ServiceDescriptorProto.verify|verify} messages.
             * @param message ServiceDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.ServiceDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ServiceDescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.ServiceDescriptorProto.verify|verify} messages.
             * @param message ServiceDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.ServiceDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a ServiceDescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.ServiceDescriptorProto & google.protobuf.ServiceDescriptorProto.$Shape} ServiceDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.ServiceDescriptorProto & google.protobuf.ServiceDescriptorProto.$Shape;

            /**
             * Decodes a ServiceDescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.ServiceDescriptorProto & google.protobuf.ServiceDescriptorProto.$Shape} ServiceDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.ServiceDescriptorProto & google.protobuf.ServiceDescriptorProto.$Shape;

            /**
             * Verifies a ServiceDescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a ServiceDescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns ServiceDescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.ServiceDescriptorProto;

            /**
             * Creates a plain object from a ServiceDescriptorProto message. Also converts values to other types if specified.
             * @param message ServiceDescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.ServiceDescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this ServiceDescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for ServiceDescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ServiceDescriptorProto {

            /** Properties of a ServiceDescriptorProto. */
            interface $Properties {

                /** ServiceDescriptorProto name */
                name?: (string|null);

                /** ServiceDescriptorProto method */
                method?: (google.protobuf.MethodDescriptorProto.$Properties[]|null);

                /** ServiceDescriptorProto options */
                options?: (google.protobuf.ServiceOptions.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a ServiceDescriptorProto. */
            type $Shape = {
              name?: string|null;
              method?: google.protobuf.MethodDescriptorProto.$Shape[]|null;
              options?: google.protobuf.ServiceOptions.$Shape|null;
              $unknowns?: Uint8Array[];
            };
        }

        /**
         * Properties of a MethodDescriptorProto.
         * @deprecated Use google.protobuf.MethodDescriptorProto.$Properties instead.
         */
        interface IMethodDescriptorProto extends google.protobuf.MethodDescriptorProto.$Properties {
        }

        /** Represents a MethodDescriptorProto. */
        class MethodDescriptorProto {

            /**
             * Constructs a new MethodDescriptorProto.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.MethodDescriptorProto.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MethodDescriptorProto name. */
            name: string;

            /** MethodDescriptorProto inputType. */
            inputType: string;

            /** MethodDescriptorProto outputType. */
            outputType: string;

            /** MethodDescriptorProto options. */
            options?: (google.protobuf.MethodOptions.$Properties|null);

            /** MethodDescriptorProto clientStreaming. */
            clientStreaming: boolean;

            /** MethodDescriptorProto serverStreaming. */
            serverStreaming: boolean;

            /**
             * Creates a new MethodDescriptorProto instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MethodDescriptorProto instance
             */
            static create(properties: google.protobuf.MethodDescriptorProto.$Shape): google.protobuf.MethodDescriptorProto & google.protobuf.MethodDescriptorProto.$Shape;
            static create(properties?: google.protobuf.MethodDescriptorProto.$Properties): google.protobuf.MethodDescriptorProto;

            /**
             * Encodes the specified MethodDescriptorProto message. Does not implicitly {@link google.protobuf.MethodDescriptorProto.verify|verify} messages.
             * @param message MethodDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.MethodDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MethodDescriptorProto message, length delimited. Does not implicitly {@link google.protobuf.MethodDescriptorProto.verify|verify} messages.
             * @param message MethodDescriptorProto message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.MethodDescriptorProto.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MethodDescriptorProto message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.MethodDescriptorProto & google.protobuf.MethodDescriptorProto.$Shape} MethodDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.MethodDescriptorProto & google.protobuf.MethodDescriptorProto.$Shape;

            /**
             * Decodes a MethodDescriptorProto message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.MethodDescriptorProto & google.protobuf.MethodDescriptorProto.$Shape} MethodDescriptorProto
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.MethodDescriptorProto & google.protobuf.MethodDescriptorProto.$Shape;

            /**
             * Verifies a MethodDescriptorProto message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a MethodDescriptorProto message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns MethodDescriptorProto
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.MethodDescriptorProto;

            /**
             * Creates a plain object from a MethodDescriptorProto message. Also converts values to other types if specified.
             * @param message MethodDescriptorProto
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.MethodDescriptorProto, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this MethodDescriptorProto to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for MethodDescriptorProto
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MethodDescriptorProto {

            /** Properties of a MethodDescriptorProto. */
            interface $Properties {

                /** MethodDescriptorProto name */
                name?: (string|null);

                /** MethodDescriptorProto inputType */
                inputType?: (string|null);

                /** MethodDescriptorProto outputType */
                outputType?: (string|null);

                /** MethodDescriptorProto options */
                options?: (google.protobuf.MethodOptions.$Properties|null);

                /** MethodDescriptorProto clientStreaming */
                clientStreaming?: (boolean|null);

                /** MethodDescriptorProto serverStreaming */
                serverStreaming?: (boolean|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MethodDescriptorProto. */
            type $Shape = {
              name?: string|null;
              inputType?: string|null;
              outputType?: string|null;
              options?: google.protobuf.MethodOptions.$Shape|null;
              clientStreaming?: boolean|null;
              serverStreaming?: boolean|null;
              $unknowns?: Uint8Array[];
            };
        }

        /**
         * Properties of a FileOptions.
         * @deprecated Use google.protobuf.FileOptions.$Properties instead.
         */
        interface IFileOptions extends google.protobuf.FileOptions.$Properties {
        }

        /** Represents a FileOptions. */
        class FileOptions {

            /**
             * Constructs a new FileOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.FileOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** FileOptions javaPackage. */
            javaPackage: string;

            /** FileOptions javaOuterClassname. */
            javaOuterClassname: string;

            /** FileOptions javaMultipleFiles. */
            javaMultipleFiles: boolean;

            /** FileOptions javaGenerateEqualsAndHash. */
            javaGenerateEqualsAndHash: boolean;

            /** FileOptions javaStringCheckUtf8. */
            javaStringCheckUtf8: boolean;

            /** FileOptions optimizeFor. */
            optimizeFor: google.protobuf.FileOptions.OptimizeMode;

            /** FileOptions goPackage. */
            goPackage: string;

            /** FileOptions ccGenericServices. */
            ccGenericServices: boolean;

            /** FileOptions javaGenericServices. */
            javaGenericServices: boolean;

            /** FileOptions pyGenericServices. */
            pyGenericServices: boolean;

            /** FileOptions phpGenericServices. */
            phpGenericServices: boolean;

            /** FileOptions deprecated. */
            deprecated: boolean;

            /** FileOptions ccEnableArenas. */
            ccEnableArenas: boolean;

            /** FileOptions objcClassPrefix. */
            objcClassPrefix: string;

            /** FileOptions csharpNamespace. */
            csharpNamespace: string;

            /** FileOptions swiftPrefix. */
            swiftPrefix: string;

            /** FileOptions phpClassPrefix. */
            phpClassPrefix: string;

            /** FileOptions phpNamespace. */
            phpNamespace: string;

            /** FileOptions phpMetadataNamespace. */
            phpMetadataNamespace: string;

            /** FileOptions rubyPackage. */
            rubyPackage: string;

            /** FileOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new FileOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns FileOptions instance
             */
            static create(properties: google.protobuf.FileOptions.$Shape): google.protobuf.FileOptions & google.protobuf.FileOptions.$Shape;
            static create(properties?: google.protobuf.FileOptions.$Properties): google.protobuf.FileOptions;

            /**
             * Encodes the specified FileOptions message. Does not implicitly {@link google.protobuf.FileOptions.verify|verify} messages.
             * @param message FileOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.FileOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified FileOptions message, length delimited. Does not implicitly {@link google.protobuf.FileOptions.verify|verify} messages.
             * @param message FileOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.FileOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a FileOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.FileOptions & google.protobuf.FileOptions.$Shape} FileOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.FileOptions & google.protobuf.FileOptions.$Shape;

            /**
             * Decodes a FileOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.FileOptions & google.protobuf.FileOptions.$Shape} FileOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.FileOptions & google.protobuf.FileOptions.$Shape;

            /**
             * Verifies a FileOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a FileOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns FileOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.FileOptions;

            /**
             * Creates a plain object from a FileOptions message. Also converts values to other types if specified.
             * @param message FileOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.FileOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this FileOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for FileOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace FileOptions {

            /** Properties of a FileOptions. */
            interface $Properties {

                /** FileOptions javaPackage */
                javaPackage?: (string|null);

                /** FileOptions javaOuterClassname */
                javaOuterClassname?: (string|null);

                /** FileOptions javaMultipleFiles */
                javaMultipleFiles?: (boolean|null);

                /** FileOptions javaGenerateEqualsAndHash */
                javaGenerateEqualsAndHash?: (boolean|null);

                /** FileOptions javaStringCheckUtf8 */
                javaStringCheckUtf8?: (boolean|null);

                /** FileOptions optimizeFor */
                optimizeFor?: (google.protobuf.FileOptions.OptimizeMode|null);

                /** FileOptions goPackage */
                goPackage?: (string|null);

                /** FileOptions ccGenericServices */
                ccGenericServices?: (boolean|null);

                /** FileOptions javaGenericServices */
                javaGenericServices?: (boolean|null);

                /** FileOptions pyGenericServices */
                pyGenericServices?: (boolean|null);

                /** FileOptions phpGenericServices */
                phpGenericServices?: (boolean|null);

                /** FileOptions deprecated */
                deprecated?: (boolean|null);

                /** FileOptions ccEnableArenas */
                ccEnableArenas?: (boolean|null);

                /** FileOptions objcClassPrefix */
                objcClassPrefix?: (string|null);

                /** FileOptions csharpNamespace */
                csharpNamespace?: (string|null);

                /** FileOptions swiftPrefix */
                swiftPrefix?: (string|null);

                /** FileOptions phpClassPrefix */
                phpClassPrefix?: (string|null);

                /** FileOptions phpNamespace */
                phpNamespace?: (string|null);

                /** FileOptions phpMetadataNamespace */
                phpMetadataNamespace?: (string|null);

                /** FileOptions rubyPackage */
                rubyPackage?: (string|null);

                /** FileOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a FileOptions. */
            type $Shape = google.protobuf.FileOptions.$Properties;

            /** OptimizeMode enum. */
            enum OptimizeMode {

                /** SPEED value */
                SPEED = 1,

                /** CODE_SIZE value */
                CODE_SIZE = 2,

                /** LITE_RUNTIME value */
                LITE_RUNTIME = 3
            }
        }

        /**
         * Properties of a MessageOptions.
         * @deprecated Use google.protobuf.MessageOptions.$Properties instead.
         */
        interface IMessageOptions extends google.protobuf.MessageOptions.$Properties {
        }

        /** Represents a MessageOptions. */
        class MessageOptions {

            /**
             * Constructs a new MessageOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.MessageOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MessageOptions messageSetWireFormat. */
            messageSetWireFormat: boolean;

            /** MessageOptions noStandardDescriptorAccessor. */
            noStandardDescriptorAccessor: boolean;

            /** MessageOptions deprecated. */
            deprecated: boolean;

            /** MessageOptions mapEntry. */
            mapEntry: boolean;

            /** MessageOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new MessageOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MessageOptions instance
             */
            static create(properties: google.protobuf.MessageOptions.$Shape): google.protobuf.MessageOptions & google.protobuf.MessageOptions.$Shape;
            static create(properties?: google.protobuf.MessageOptions.$Properties): google.protobuf.MessageOptions;

            /**
             * Encodes the specified MessageOptions message. Does not implicitly {@link google.protobuf.MessageOptions.verify|verify} messages.
             * @param message MessageOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.MessageOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MessageOptions message, length delimited. Does not implicitly {@link google.protobuf.MessageOptions.verify|verify} messages.
             * @param message MessageOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.MessageOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MessageOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.MessageOptions & google.protobuf.MessageOptions.$Shape} MessageOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.MessageOptions & google.protobuf.MessageOptions.$Shape;

            /**
             * Decodes a MessageOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.MessageOptions & google.protobuf.MessageOptions.$Shape} MessageOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.MessageOptions & google.protobuf.MessageOptions.$Shape;

            /**
             * Verifies a MessageOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a MessageOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns MessageOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.MessageOptions;

            /**
             * Creates a plain object from a MessageOptions message. Also converts values to other types if specified.
             * @param message MessageOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.MessageOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this MessageOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for MessageOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MessageOptions {

            /** Properties of a MessageOptions. */
            interface $Properties {

                /** MessageOptions messageSetWireFormat */
                messageSetWireFormat?: (boolean|null);

                /** MessageOptions noStandardDescriptorAccessor */
                noStandardDescriptorAccessor?: (boolean|null);

                /** MessageOptions deprecated */
                deprecated?: (boolean|null);

                /** MessageOptions mapEntry */
                mapEntry?: (boolean|null);

                /** MessageOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MessageOptions. */
            type $Shape = google.protobuf.MessageOptions.$Properties;
        }

        /**
         * Properties of a FieldOptions.
         * @deprecated Use google.protobuf.FieldOptions.$Properties instead.
         */
        interface IFieldOptions extends google.protobuf.FieldOptions.$Properties {
        }

        /** Represents a FieldOptions. */
        class FieldOptions {

            /**
             * Constructs a new FieldOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.FieldOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** FieldOptions ctype. */
            ctype: google.protobuf.FieldOptions.CType;

            /** FieldOptions packed. */
            packed: boolean;

            /** FieldOptions jstype. */
            jstype: google.protobuf.FieldOptions.JSType;

            /** FieldOptions lazy. */
            lazy: boolean;

            /** FieldOptions unverifiedLazy. */
            unverifiedLazy: boolean;

            /** FieldOptions deprecated. */
            deprecated: boolean;

            /** FieldOptions weak. */
            weak: boolean;

            /** FieldOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new FieldOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns FieldOptions instance
             */
            static create(properties: google.protobuf.FieldOptions.$Shape): google.protobuf.FieldOptions & google.protobuf.FieldOptions.$Shape;
            static create(properties?: google.protobuf.FieldOptions.$Properties): google.protobuf.FieldOptions;

            /**
             * Encodes the specified FieldOptions message. Does not implicitly {@link google.protobuf.FieldOptions.verify|verify} messages.
             * @param message FieldOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.FieldOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified FieldOptions message, length delimited. Does not implicitly {@link google.protobuf.FieldOptions.verify|verify} messages.
             * @param message FieldOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.FieldOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a FieldOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.FieldOptions & google.protobuf.FieldOptions.$Shape} FieldOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.FieldOptions & google.protobuf.FieldOptions.$Shape;

            /**
             * Decodes a FieldOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.FieldOptions & google.protobuf.FieldOptions.$Shape} FieldOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.FieldOptions & google.protobuf.FieldOptions.$Shape;

            /**
             * Verifies a FieldOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a FieldOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns FieldOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.FieldOptions;

            /**
             * Creates a plain object from a FieldOptions message. Also converts values to other types if specified.
             * @param message FieldOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.FieldOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this FieldOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for FieldOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace FieldOptions {

            /** Properties of a FieldOptions. */
            interface $Properties {

                /** FieldOptions ctype */
                ctype?: (google.protobuf.FieldOptions.CType|null);

                /** FieldOptions packed */
                packed?: (boolean|null);

                /** FieldOptions jstype */
                jstype?: (google.protobuf.FieldOptions.JSType|null);

                /** FieldOptions lazy */
                lazy?: (boolean|null);

                /** FieldOptions unverifiedLazy */
                unverifiedLazy?: (boolean|null);

                /** FieldOptions deprecated */
                deprecated?: (boolean|null);

                /** FieldOptions weak */
                weak?: (boolean|null);

                /** FieldOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a FieldOptions. */
            type $Shape = google.protobuf.FieldOptions.$Properties;

            /** CType enum. */
            enum CType {

                /** STRING value */
                STRING = 0,

                /** CORD value */
                CORD = 1,

                /** STRING_PIECE value */
                STRING_PIECE = 2
            }

            /** JSType enum. */
            enum JSType {

                /** JS_NORMAL value */
                JS_NORMAL = 0,

                /** JS_STRING value */
                JS_STRING = 1,

                /** JS_NUMBER value */
                JS_NUMBER = 2
            }
        }

        /**
         * Properties of a OneofOptions.
         * @deprecated Use google.protobuf.OneofOptions.$Properties instead.
         */
        interface IOneofOptions extends google.protobuf.OneofOptions.$Properties {
        }

        /** Represents a OneofOptions. */
        class OneofOptions {

            /**
             * Constructs a new OneofOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.OneofOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** OneofOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new OneofOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns OneofOptions instance
             */
            static create(properties: google.protobuf.OneofOptions.$Shape): google.protobuf.OneofOptions & google.protobuf.OneofOptions.$Shape;
            static create(properties?: google.protobuf.OneofOptions.$Properties): google.protobuf.OneofOptions;

            /**
             * Encodes the specified OneofOptions message. Does not implicitly {@link google.protobuf.OneofOptions.verify|verify} messages.
             * @param message OneofOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.OneofOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified OneofOptions message, length delimited. Does not implicitly {@link google.protobuf.OneofOptions.verify|verify} messages.
             * @param message OneofOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.OneofOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a OneofOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.OneofOptions & google.protobuf.OneofOptions.$Shape} OneofOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.OneofOptions & google.protobuf.OneofOptions.$Shape;

            /**
             * Decodes a OneofOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.OneofOptions & google.protobuf.OneofOptions.$Shape} OneofOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.OneofOptions & google.protobuf.OneofOptions.$Shape;

            /**
             * Verifies a OneofOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a OneofOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns OneofOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.OneofOptions;

            /**
             * Creates a plain object from a OneofOptions message. Also converts values to other types if specified.
             * @param message OneofOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.OneofOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this OneofOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for OneofOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace OneofOptions {

            /** Properties of a OneofOptions. */
            interface $Properties {

                /** OneofOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a OneofOptions. */
            type $Shape = google.protobuf.OneofOptions.$Properties;
        }

        /**
         * Properties of an EnumOptions.
         * @deprecated Use google.protobuf.EnumOptions.$Properties instead.
         */
        interface IEnumOptions extends google.protobuf.EnumOptions.$Properties {
        }

        /** Represents an EnumOptions. */
        class EnumOptions {

            /**
             * Constructs a new EnumOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.EnumOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EnumOptions allowAlias. */
            allowAlias: boolean;

            /** EnumOptions deprecated. */
            deprecated: boolean;

            /** EnumOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new EnumOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EnumOptions instance
             */
            static create(properties: google.protobuf.EnumOptions.$Shape): google.protobuf.EnumOptions & google.protobuf.EnumOptions.$Shape;
            static create(properties?: google.protobuf.EnumOptions.$Properties): google.protobuf.EnumOptions;

            /**
             * Encodes the specified EnumOptions message. Does not implicitly {@link google.protobuf.EnumOptions.verify|verify} messages.
             * @param message EnumOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.EnumOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EnumOptions message, length delimited. Does not implicitly {@link google.protobuf.EnumOptions.verify|verify} messages.
             * @param message EnumOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.EnumOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EnumOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.EnumOptions & google.protobuf.EnumOptions.$Shape} EnumOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.EnumOptions & google.protobuf.EnumOptions.$Shape;

            /**
             * Decodes an EnumOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.EnumOptions & google.protobuf.EnumOptions.$Shape} EnumOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.EnumOptions & google.protobuf.EnumOptions.$Shape;

            /**
             * Verifies an EnumOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates an EnumOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns EnumOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.EnumOptions;

            /**
             * Creates a plain object from an EnumOptions message. Also converts values to other types if specified.
             * @param message EnumOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.EnumOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this EnumOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for EnumOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EnumOptions {

            /** Properties of an EnumOptions. */
            interface $Properties {

                /** EnumOptions allowAlias */
                allowAlias?: (boolean|null);

                /** EnumOptions deprecated */
                deprecated?: (boolean|null);

                /** EnumOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EnumOptions. */
            type $Shape = google.protobuf.EnumOptions.$Properties;
        }

        /**
         * Properties of an EnumValueOptions.
         * @deprecated Use google.protobuf.EnumValueOptions.$Properties instead.
         */
        interface IEnumValueOptions extends google.protobuf.EnumValueOptions.$Properties {
        }

        /** Represents an EnumValueOptions. */
        class EnumValueOptions {

            /**
             * Constructs a new EnumValueOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.EnumValueOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** EnumValueOptions deprecated. */
            deprecated: boolean;

            /** EnumValueOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new EnumValueOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns EnumValueOptions instance
             */
            static create(properties: google.protobuf.EnumValueOptions.$Shape): google.protobuf.EnumValueOptions & google.protobuf.EnumValueOptions.$Shape;
            static create(properties?: google.protobuf.EnumValueOptions.$Properties): google.protobuf.EnumValueOptions;

            /**
             * Encodes the specified EnumValueOptions message. Does not implicitly {@link google.protobuf.EnumValueOptions.verify|verify} messages.
             * @param message EnumValueOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.EnumValueOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified EnumValueOptions message, length delimited. Does not implicitly {@link google.protobuf.EnumValueOptions.verify|verify} messages.
             * @param message EnumValueOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.EnumValueOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an EnumValueOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.EnumValueOptions & google.protobuf.EnumValueOptions.$Shape} EnumValueOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.EnumValueOptions & google.protobuf.EnumValueOptions.$Shape;

            /**
             * Decodes an EnumValueOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.EnumValueOptions & google.protobuf.EnumValueOptions.$Shape} EnumValueOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.EnumValueOptions & google.protobuf.EnumValueOptions.$Shape;

            /**
             * Verifies an EnumValueOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates an EnumValueOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns EnumValueOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.EnumValueOptions;

            /**
             * Creates a plain object from an EnumValueOptions message. Also converts values to other types if specified.
             * @param message EnumValueOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.EnumValueOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this EnumValueOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for EnumValueOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace EnumValueOptions {

            /** Properties of an EnumValueOptions. */
            interface $Properties {

                /** EnumValueOptions deprecated */
                deprecated?: (boolean|null);

                /** EnumValueOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an EnumValueOptions. */
            type $Shape = google.protobuf.EnumValueOptions.$Properties;
        }

        /**
         * Properties of a ServiceOptions.
         * @deprecated Use google.protobuf.ServiceOptions.$Properties instead.
         */
        interface IServiceOptions extends google.protobuf.ServiceOptions.$Properties {
        }

        /** Represents a ServiceOptions. */
        class ServiceOptions {

            /**
             * Constructs a new ServiceOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.ServiceOptions.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** ServiceOptions deprecated. */
            deprecated: boolean;

            /** ServiceOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new ServiceOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns ServiceOptions instance
             */
            static create(properties: google.protobuf.ServiceOptions.$Shape): google.protobuf.ServiceOptions & google.protobuf.ServiceOptions.$Shape;
            static create(properties?: google.protobuf.ServiceOptions.$Properties): google.protobuf.ServiceOptions;

            /**
             * Encodes the specified ServiceOptions message. Does not implicitly {@link google.protobuf.ServiceOptions.verify|verify} messages.
             * @param message ServiceOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.ServiceOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified ServiceOptions message, length delimited. Does not implicitly {@link google.protobuf.ServiceOptions.verify|verify} messages.
             * @param message ServiceOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.ServiceOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a ServiceOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.ServiceOptions & google.protobuf.ServiceOptions.$Shape} ServiceOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.ServiceOptions & google.protobuf.ServiceOptions.$Shape;

            /**
             * Decodes a ServiceOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.ServiceOptions & google.protobuf.ServiceOptions.$Shape} ServiceOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.ServiceOptions & google.protobuf.ServiceOptions.$Shape;

            /**
             * Verifies a ServiceOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a ServiceOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns ServiceOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.ServiceOptions;

            /**
             * Creates a plain object from a ServiceOptions message. Also converts values to other types if specified.
             * @param message ServiceOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.ServiceOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this ServiceOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for ServiceOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace ServiceOptions {

            /** Properties of a ServiceOptions. */
            interface $Properties {

                /** ServiceOptions deprecated */
                deprecated?: (boolean|null);

                /** ServiceOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a ServiceOptions. */
            type $Shape = google.protobuf.ServiceOptions.$Properties;
        }

        /**
         * Properties of a MethodOptions.
         * @deprecated Use google.protobuf.MethodOptions.$Properties instead.
         */
        interface IMethodOptions extends google.protobuf.MethodOptions.$Properties {
        }

        /** Represents a MethodOptions. */
        class MethodOptions {

            /**
             * Constructs a new MethodOptions.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.MethodOptions.$Properties);

            /** MethodOptions .google.api.http */
            ".google.api.http"?: (google.api.HttpRule.$Properties|null);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** MethodOptions deprecated. */
            deprecated: boolean;

            /** MethodOptions idempotencyLevel. */
            idempotencyLevel: google.protobuf.MethodOptions.IdempotencyLevel;

            /** MethodOptions uninterpretedOption. */
            uninterpretedOption: google.protobuf.UninterpretedOption.$Properties[];

            /**
             * Creates a new MethodOptions instance using the specified properties.
             * @param [properties] Properties to set
             * @returns MethodOptions instance
             */
            static create(properties: google.protobuf.MethodOptions.$Shape): google.protobuf.MethodOptions & google.protobuf.MethodOptions.$Shape;
            static create(properties?: google.protobuf.MethodOptions.$Properties): google.protobuf.MethodOptions;

            /**
             * Encodes the specified MethodOptions message. Does not implicitly {@link google.protobuf.MethodOptions.verify|verify} messages.
             * @param message MethodOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.MethodOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified MethodOptions message, length delimited. Does not implicitly {@link google.protobuf.MethodOptions.verify|verify} messages.
             * @param message MethodOptions message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.MethodOptions.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a MethodOptions message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.MethodOptions & google.protobuf.MethodOptions.$Shape} MethodOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.MethodOptions & google.protobuf.MethodOptions.$Shape;

            /**
             * Decodes a MethodOptions message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.MethodOptions & google.protobuf.MethodOptions.$Shape} MethodOptions
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.MethodOptions & google.protobuf.MethodOptions.$Shape;

            /**
             * Verifies a MethodOptions message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a MethodOptions message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns MethodOptions
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.MethodOptions;

            /**
             * Creates a plain object from a MethodOptions message. Also converts values to other types if specified.
             * @param message MethodOptions
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.MethodOptions, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this MethodOptions to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for MethodOptions
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace MethodOptions {

            /** Properties of a MethodOptions. */
            interface $Properties {

                /** MethodOptions deprecated */
                deprecated?: (boolean|null);

                /** MethodOptions idempotencyLevel */
                idempotencyLevel?: (google.protobuf.MethodOptions.IdempotencyLevel|null);

                /** MethodOptions uninterpretedOption */
                uninterpretedOption?: (google.protobuf.UninterpretedOption.$Properties[]|null);

                /** MethodOptions .google.api.http */
                ".google.api.http"?: (google.api.HttpRule.$Properties|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a MethodOptions. */
            type $Shape = {
              deprecated?: boolean|null;
              idempotencyLevel?: google.protobuf.MethodOptions.IdempotencyLevel|null;
              uninterpretedOption?: google.protobuf.UninterpretedOption.$Shape[]|null;
              ".google.api.http"?: google.api.HttpRule.$Shape|null;
              $unknowns?: Uint8Array[];
            };

            /** IdempotencyLevel enum. */
            enum IdempotencyLevel {

                /** IDEMPOTENCY_UNKNOWN value */
                IDEMPOTENCY_UNKNOWN = 0,

                /** NO_SIDE_EFFECTS value */
                NO_SIDE_EFFECTS = 1,

                /** IDEMPOTENT value */
                IDEMPOTENT = 2
            }
        }

        /**
         * Properties of an UninterpretedOption.
         * @deprecated Use google.protobuf.UninterpretedOption.$Properties instead.
         */
        interface IUninterpretedOption extends google.protobuf.UninterpretedOption.$Properties {
        }

        /** Represents an UninterpretedOption. */
        class UninterpretedOption {

            /**
             * Constructs a new UninterpretedOption.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.UninterpretedOption.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** UninterpretedOption name. */
            name: google.protobuf.UninterpretedOption.NamePart.$Properties[];

            /** UninterpretedOption identifierValue. */
            identifierValue: string;

            /** UninterpretedOption positiveIntValue. */
            positiveIntValue: (number|Long);

            /** UninterpretedOption negativeIntValue. */
            negativeIntValue: (number|Long);

            /** UninterpretedOption doubleValue. */
            doubleValue: number;

            /** UninterpretedOption stringValue. */
            stringValue: Uint8Array;

            /** UninterpretedOption aggregateValue. */
            aggregateValue: string;

            /**
             * Creates a new UninterpretedOption instance using the specified properties.
             * @param [properties] Properties to set
             * @returns UninterpretedOption instance
             */
            static create(properties: google.protobuf.UninterpretedOption.$Shape): google.protobuf.UninterpretedOption & google.protobuf.UninterpretedOption.$Shape;
            static create(properties?: google.protobuf.UninterpretedOption.$Properties): google.protobuf.UninterpretedOption;

            /**
             * Encodes the specified UninterpretedOption message. Does not implicitly {@link google.protobuf.UninterpretedOption.verify|verify} messages.
             * @param message UninterpretedOption message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.UninterpretedOption.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified UninterpretedOption message, length delimited. Does not implicitly {@link google.protobuf.UninterpretedOption.verify|verify} messages.
             * @param message UninterpretedOption message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.UninterpretedOption.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes an UninterpretedOption message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.UninterpretedOption & google.protobuf.UninterpretedOption.$Shape} UninterpretedOption
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.UninterpretedOption & google.protobuf.UninterpretedOption.$Shape;

            /**
             * Decodes an UninterpretedOption message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.UninterpretedOption & google.protobuf.UninterpretedOption.$Shape} UninterpretedOption
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.UninterpretedOption & google.protobuf.UninterpretedOption.$Shape;

            /**
             * Verifies an UninterpretedOption message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates an UninterpretedOption message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns UninterpretedOption
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.UninterpretedOption;

            /**
             * Creates a plain object from an UninterpretedOption message. Also converts values to other types if specified.
             * @param message UninterpretedOption
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.UninterpretedOption, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this UninterpretedOption to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for UninterpretedOption
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace UninterpretedOption {

            /** Properties of an UninterpretedOption. */
            interface $Properties {

                /** UninterpretedOption name */
                name?: (google.protobuf.UninterpretedOption.NamePart.$Properties[]|null);

                /** UninterpretedOption identifierValue */
                identifierValue?: (string|null);

                /** UninterpretedOption positiveIntValue */
                positiveIntValue?: (number|Long|null);

                /** UninterpretedOption negativeIntValue */
                negativeIntValue?: (number|Long|null);

                /** UninterpretedOption doubleValue */
                doubleValue?: (number|null);

                /** UninterpretedOption stringValue */
                stringValue?: (Uint8Array|null);

                /** UninterpretedOption aggregateValue */
                aggregateValue?: (string|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of an UninterpretedOption. */
            type $Shape = google.protobuf.UninterpretedOption.$Properties;

            /**
             * Properties of a NamePart.
             * @deprecated Use google.protobuf.UninterpretedOption.NamePart.$Properties instead.
             */
            interface INamePart extends google.protobuf.UninterpretedOption.NamePart.$Properties {
            }

            /** Represents a NamePart. */
            class NamePart {

                /**
                 * Constructs a new NamePart.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: google.protobuf.UninterpretedOption.NamePart.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** NamePart namePart. */
                namePart: string;

                /** NamePart isExtension. */
                isExtension: boolean;

                /**
                 * Creates a new NamePart instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns NamePart instance
                 */
                static create(properties: google.protobuf.UninterpretedOption.NamePart.$Shape): google.protobuf.UninterpretedOption.NamePart & google.protobuf.UninterpretedOption.NamePart.$Shape;
                static create(properties?: google.protobuf.UninterpretedOption.NamePart.$Properties): google.protobuf.UninterpretedOption.NamePart;

                /**
                 * Encodes the specified NamePart message. Does not implicitly {@link google.protobuf.UninterpretedOption.NamePart.verify|verify} messages.
                 * @param message NamePart message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: google.protobuf.UninterpretedOption.NamePart.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified NamePart message, length delimited. Does not implicitly {@link google.protobuf.UninterpretedOption.NamePart.verify|verify} messages.
                 * @param message NamePart message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: google.protobuf.UninterpretedOption.NamePart.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a NamePart message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {google.protobuf.UninterpretedOption.NamePart & google.protobuf.UninterpretedOption.NamePart.$Shape} NamePart
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.UninterpretedOption.NamePart & google.protobuf.UninterpretedOption.NamePart.$Shape;

                /**
                 * Decodes a NamePart message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {google.protobuf.UninterpretedOption.NamePart & google.protobuf.UninterpretedOption.NamePart.$Shape} NamePart
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.UninterpretedOption.NamePart & google.protobuf.UninterpretedOption.NamePart.$Shape;

                /**
                 * Verifies a NamePart message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a NamePart message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns NamePart
                 */
                static fromObject(object: { [k: string]: any }): google.protobuf.UninterpretedOption.NamePart;

                /**
                 * Creates a plain object from a NamePart message. Also converts values to other types if specified.
                 * @param message NamePart
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: google.protobuf.UninterpretedOption.NamePart, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this NamePart to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for NamePart
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace NamePart {

                /** Properties of a NamePart. */
                interface $Properties {

                    /** NamePart namePart */
                    namePart: string;

                    /** NamePart isExtension */
                    isExtension: boolean;

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a NamePart. */
                type $Shape = google.protobuf.UninterpretedOption.NamePart.$Properties;
            }
        }

        /**
         * Properties of a SourceCodeInfo.
         * @deprecated Use google.protobuf.SourceCodeInfo.$Properties instead.
         */
        interface ISourceCodeInfo extends google.protobuf.SourceCodeInfo.$Properties {
        }

        /** Represents a SourceCodeInfo. */
        class SourceCodeInfo {

            /**
             * Constructs a new SourceCodeInfo.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.SourceCodeInfo.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** SourceCodeInfo location. */
            location: google.protobuf.SourceCodeInfo.Location.$Properties[];

            /**
             * Creates a new SourceCodeInfo instance using the specified properties.
             * @param [properties] Properties to set
             * @returns SourceCodeInfo instance
             */
            static create(properties: google.protobuf.SourceCodeInfo.$Shape): google.protobuf.SourceCodeInfo & google.protobuf.SourceCodeInfo.$Shape;
            static create(properties?: google.protobuf.SourceCodeInfo.$Properties): google.protobuf.SourceCodeInfo;

            /**
             * Encodes the specified SourceCodeInfo message. Does not implicitly {@link google.protobuf.SourceCodeInfo.verify|verify} messages.
             * @param message SourceCodeInfo message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.SourceCodeInfo.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified SourceCodeInfo message, length delimited. Does not implicitly {@link google.protobuf.SourceCodeInfo.verify|verify} messages.
             * @param message SourceCodeInfo message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.SourceCodeInfo.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a SourceCodeInfo message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.SourceCodeInfo & google.protobuf.SourceCodeInfo.$Shape} SourceCodeInfo
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.SourceCodeInfo & google.protobuf.SourceCodeInfo.$Shape;

            /**
             * Decodes a SourceCodeInfo message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.SourceCodeInfo & google.protobuf.SourceCodeInfo.$Shape} SourceCodeInfo
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.SourceCodeInfo & google.protobuf.SourceCodeInfo.$Shape;

            /**
             * Verifies a SourceCodeInfo message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a SourceCodeInfo message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns SourceCodeInfo
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.SourceCodeInfo;

            /**
             * Creates a plain object from a SourceCodeInfo message. Also converts values to other types if specified.
             * @param message SourceCodeInfo
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.SourceCodeInfo, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this SourceCodeInfo to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for SourceCodeInfo
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace SourceCodeInfo {

            /** Properties of a SourceCodeInfo. */
            interface $Properties {

                /** SourceCodeInfo location */
                location?: (google.protobuf.SourceCodeInfo.Location.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a SourceCodeInfo. */
            type $Shape = google.protobuf.SourceCodeInfo.$Properties;

            /**
             * Properties of a Location.
             * @deprecated Use google.protobuf.SourceCodeInfo.Location.$Properties instead.
             */
            interface ILocation extends google.protobuf.SourceCodeInfo.Location.$Properties {
            }

            /** Represents a Location. */
            class Location {

                /**
                 * Constructs a new Location.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: google.protobuf.SourceCodeInfo.Location.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** Location path. */
                path: number[];

                /** Location span. */
                span: number[];

                /** Location leadingComments. */
                leadingComments: string;

                /** Location trailingComments. */
                trailingComments: string;

                /** Location leadingDetachedComments. */
                leadingDetachedComments: string[];

                /**
                 * Creates a new Location instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns Location instance
                 */
                static create(properties: google.protobuf.SourceCodeInfo.Location.$Shape): google.protobuf.SourceCodeInfo.Location & google.protobuf.SourceCodeInfo.Location.$Shape;
                static create(properties?: google.protobuf.SourceCodeInfo.Location.$Properties): google.protobuf.SourceCodeInfo.Location;

                /**
                 * Encodes the specified Location message. Does not implicitly {@link google.protobuf.SourceCodeInfo.Location.verify|verify} messages.
                 * @param message Location message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: google.protobuf.SourceCodeInfo.Location.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified Location message, length delimited. Does not implicitly {@link google.protobuf.SourceCodeInfo.Location.verify|verify} messages.
                 * @param message Location message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: google.protobuf.SourceCodeInfo.Location.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes a Location message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {google.protobuf.SourceCodeInfo.Location & google.protobuf.SourceCodeInfo.Location.$Shape} Location
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.SourceCodeInfo.Location & google.protobuf.SourceCodeInfo.Location.$Shape;

                /**
                 * Decodes a Location message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {google.protobuf.SourceCodeInfo.Location & google.protobuf.SourceCodeInfo.Location.$Shape} Location
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.SourceCodeInfo.Location & google.protobuf.SourceCodeInfo.Location.$Shape;

                /**
                 * Verifies a Location message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates a Location message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns Location
                 */
                static fromObject(object: { [k: string]: any }): google.protobuf.SourceCodeInfo.Location;

                /**
                 * Creates a plain object from a Location message. Also converts values to other types if specified.
                 * @param message Location
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: google.protobuf.SourceCodeInfo.Location, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this Location to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for Location
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace Location {

                /** Properties of a Location. */
                interface $Properties {

                    /** Location path */
                    path?: (number[]|null);

                    /** Location span */
                    span?: (number[]|null);

                    /** Location leadingComments */
                    leadingComments?: (string|null);

                    /** Location trailingComments */
                    trailingComments?: (string|null);

                    /** Location leadingDetachedComments */
                    leadingDetachedComments?: (string[]|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of a Location. */
                type $Shape = google.protobuf.SourceCodeInfo.Location.$Properties;
            }
        }

        /**
         * Properties of a GeneratedCodeInfo.
         * @deprecated Use google.protobuf.GeneratedCodeInfo.$Properties instead.
         */
        interface IGeneratedCodeInfo extends google.protobuf.GeneratedCodeInfo.$Properties {
        }

        /** Represents a GeneratedCodeInfo. */
        class GeneratedCodeInfo {

            /**
             * Constructs a new GeneratedCodeInfo.
             * @param [properties] Properties to set
             */
            constructor(properties?: google.protobuf.GeneratedCodeInfo.$Properties);

            /** Unknown fields preserved while decoding when enabled */
            $unknowns?: Uint8Array[];

            /** GeneratedCodeInfo annotation. */
            annotation: google.protobuf.GeneratedCodeInfo.Annotation.$Properties[];

            /**
             * Creates a new GeneratedCodeInfo instance using the specified properties.
             * @param [properties] Properties to set
             * @returns GeneratedCodeInfo instance
             */
            static create(properties: google.protobuf.GeneratedCodeInfo.$Shape): google.protobuf.GeneratedCodeInfo & google.protobuf.GeneratedCodeInfo.$Shape;
            static create(properties?: google.protobuf.GeneratedCodeInfo.$Properties): google.protobuf.GeneratedCodeInfo;

            /**
             * Encodes the specified GeneratedCodeInfo message. Does not implicitly {@link google.protobuf.GeneratedCodeInfo.verify|verify} messages.
             * @param message GeneratedCodeInfo message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encode(message: google.protobuf.GeneratedCodeInfo.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Encodes the specified GeneratedCodeInfo message, length delimited. Does not implicitly {@link google.protobuf.GeneratedCodeInfo.verify|verify} messages.
             * @param message GeneratedCodeInfo message or plain object to encode
             * @param [writer] Writer to encode to
             * @returns Writer
             */
            static encodeDelimited(message: google.protobuf.GeneratedCodeInfo.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

            /**
             * Decodes a GeneratedCodeInfo message from the specified reader or buffer.
             * @param reader Reader or buffer to decode from
             * @param [length] Message length if known beforehand
             * @returns {google.protobuf.GeneratedCodeInfo & google.protobuf.GeneratedCodeInfo.$Shape} GeneratedCodeInfo
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.GeneratedCodeInfo & google.protobuf.GeneratedCodeInfo.$Shape;

            /**
             * Decodes a GeneratedCodeInfo message from the specified reader or buffer, length delimited.
             * @param reader Reader or buffer to decode from
             * @returns {google.protobuf.GeneratedCodeInfo & google.protobuf.GeneratedCodeInfo.$Shape} GeneratedCodeInfo
             * @throws {Error} If the payload is not a reader or valid buffer
             * @throws {$protobuf.util.ProtocolError} If required fields are missing
             */
            static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.GeneratedCodeInfo & google.protobuf.GeneratedCodeInfo.$Shape;

            /**
             * Verifies a GeneratedCodeInfo message.
             * @param message Plain object to verify
             * @returns `null` if valid, otherwise the reason why it is not
             */
            static verify(message: { [k: string]: any }): (string|null);

            /**
             * Creates a GeneratedCodeInfo message from a plain object. Also converts values to their respective internal types.
             * @param object Plain object
             * @returns GeneratedCodeInfo
             */
            static fromObject(object: { [k: string]: any }): google.protobuf.GeneratedCodeInfo;

            /**
             * Creates a plain object from a GeneratedCodeInfo message. Also converts values to other types if specified.
             * @param message GeneratedCodeInfo
             * @param [options] Conversion options
             * @returns Plain object
             */
            static toObject(message: google.protobuf.GeneratedCodeInfo, options?: $protobuf.IConversionOptions): { [k: string]: any };

            /**
             * Converts this GeneratedCodeInfo to JSON.
             * @returns JSON object
             */
            toJSON(): { [k: string]: any };

            /**
             * Gets the type url for GeneratedCodeInfo
             * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
             * @returns The type url
             */
            static getTypeUrl(prefix?: string): string;
        }

        namespace GeneratedCodeInfo {

            /** Properties of a GeneratedCodeInfo. */
            interface $Properties {

                /** GeneratedCodeInfo annotation */
                annotation?: (google.protobuf.GeneratedCodeInfo.Annotation.$Properties[]|null);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];
            }

            /** Shape of a GeneratedCodeInfo. */
            type $Shape = google.protobuf.GeneratedCodeInfo.$Properties;

            /**
             * Properties of an Annotation.
             * @deprecated Use google.protobuf.GeneratedCodeInfo.Annotation.$Properties instead.
             */
            interface IAnnotation extends google.protobuf.GeneratedCodeInfo.Annotation.$Properties {
            }

            /** Represents an Annotation. */
            class Annotation {

                /**
                 * Constructs a new Annotation.
                 * @param [properties] Properties to set
                 */
                constructor(properties?: google.protobuf.GeneratedCodeInfo.Annotation.$Properties);

                /** Unknown fields preserved while decoding when enabled */
                $unknowns?: Uint8Array[];

                /** Annotation path. */
                path: number[];

                /** Annotation sourceFile. */
                sourceFile: string;

                /** Annotation begin. */
                begin: number;

                /** Annotation end. */
                end: number;

                /**
                 * Creates a new Annotation instance using the specified properties.
                 * @param [properties] Properties to set
                 * @returns Annotation instance
                 */
                static create(properties: google.protobuf.GeneratedCodeInfo.Annotation.$Shape): google.protobuf.GeneratedCodeInfo.Annotation & google.protobuf.GeneratedCodeInfo.Annotation.$Shape;
                static create(properties?: google.protobuf.GeneratedCodeInfo.Annotation.$Properties): google.protobuf.GeneratedCodeInfo.Annotation;

                /**
                 * Encodes the specified Annotation message. Does not implicitly {@link google.protobuf.GeneratedCodeInfo.Annotation.verify|verify} messages.
                 * @param message Annotation message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encode(message: google.protobuf.GeneratedCodeInfo.Annotation.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Encodes the specified Annotation message, length delimited. Does not implicitly {@link google.protobuf.GeneratedCodeInfo.Annotation.verify|verify} messages.
                 * @param message Annotation message or plain object to encode
                 * @param [writer] Writer to encode to
                 * @returns Writer
                 */
                static encodeDelimited(message: google.protobuf.GeneratedCodeInfo.Annotation.$Properties, writer?: $protobuf.Writer): $protobuf.Writer;

                /**
                 * Decodes an Annotation message from the specified reader or buffer.
                 * @param reader Reader or buffer to decode from
                 * @param [length] Message length if known beforehand
                 * @returns {google.protobuf.GeneratedCodeInfo.Annotation & google.protobuf.GeneratedCodeInfo.Annotation.$Shape} Annotation
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decode(reader: ($protobuf.Reader|Uint8Array), length?: number): google.protobuf.GeneratedCodeInfo.Annotation & google.protobuf.GeneratedCodeInfo.Annotation.$Shape;

                /**
                 * Decodes an Annotation message from the specified reader or buffer, length delimited.
                 * @param reader Reader or buffer to decode from
                 * @returns {google.protobuf.GeneratedCodeInfo.Annotation & google.protobuf.GeneratedCodeInfo.Annotation.$Shape} Annotation
                 * @throws {Error} If the payload is not a reader or valid buffer
                 * @throws {$protobuf.util.ProtocolError} If required fields are missing
                 */
                static decodeDelimited(reader: ($protobuf.Reader|Uint8Array)): google.protobuf.GeneratedCodeInfo.Annotation & google.protobuf.GeneratedCodeInfo.Annotation.$Shape;

                /**
                 * Verifies an Annotation message.
                 * @param message Plain object to verify
                 * @returns `null` if valid, otherwise the reason why it is not
                 */
                static verify(message: { [k: string]: any }): (string|null);

                /**
                 * Creates an Annotation message from a plain object. Also converts values to their respective internal types.
                 * @param object Plain object
                 * @returns Annotation
                 */
                static fromObject(object: { [k: string]: any }): google.protobuf.GeneratedCodeInfo.Annotation;

                /**
                 * Creates a plain object from an Annotation message. Also converts values to other types if specified.
                 * @param message Annotation
                 * @param [options] Conversion options
                 * @returns Plain object
                 */
                static toObject(message: google.protobuf.GeneratedCodeInfo.Annotation, options?: $protobuf.IConversionOptions): { [k: string]: any };

                /**
                 * Converts this Annotation to JSON.
                 * @returns JSON object
                 */
                toJSON(): { [k: string]: any };

                /**
                 * Gets the type url for Annotation
                 * @param [prefix] Custom type url prefix, defaults to `"type.googleapis.com"`
                 * @returns The type url
                 */
                static getTypeUrl(prefix?: string): string;
            }

            namespace Annotation {

                /** Properties of an Annotation. */
                interface $Properties {

                    /** Annotation path */
                    path?: (number[]|null);

                    /** Annotation sourceFile */
                    sourceFile?: (string|null);

                    /** Annotation begin */
                    begin?: (number|null);

                    /** Annotation end */
                    end?: (number|null);

                    /** Unknown fields preserved while decoding when enabled */
                    $unknowns?: Uint8Array[];
                }

                /** Shape of an Annotation. */
                type $Shape = google.protobuf.GeneratedCodeInfo.Annotation.$Properties;
            }
        }
    }
}
