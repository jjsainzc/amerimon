# Configuración PJSIP en Asterisk (`pjsip.conf`)

Estas opciones forman parte de la configuración de un endpoint (extremo, como un teléfono IP o una troncal VoIP) en el archivo **`pjsip.conf`** de **Asterisk**. Cada una de estas variables controla cómo Asterisk maneja el audio, el video, la red y el NAT para ese dispositivo o troncal.

---

## 1. Manejo de Red y NAT

* **`rtp_symmetric=yes`**
  * **Qué hace:** Fuerza a Asterisk a enviar los paquetes de audio/video (RTP) de vuelta a la *misma dirección IP y puerto* exactos desde los cuales el dispositivo se los envió originalmente.
  * **Para qué sirve:** Es fundamental para evitar problemas de **NAT**, asegurando que el flujo de medios no se pierda en el enrutamiento del router.

* **`force_rport=yes`**
  * **Qué hace:** Indica a Asterisk que ignore el puerto que el teléfono dice usar en sus mensajes SIP internos y que obligatoriamente responda al **puerto real (remote port)** desde donde llegó el paquete a nivel de red (capa UDP).
  * **Para qué sirve:** Crucial para dispositivos detrás de un router NAT, ya que los routers suelen cambiar los puertos de origen al salir a internet.

* **`rewrite_contact=yes`**
  * **Qué hace:** Obliga a Asterisk a sobrescribir la dirección IP y el puerto contenidos en la cabecera `Contact` del mensaje SIP de registro con la dirección IP y el puerto reales desde los cuales Asterisk recibió el paquete SIP.
  * **Para qué sirve:** Soluciona problemas comunes donde el teléfono IP se registra enviando su IP local (ej. `192.168.1.50`) en lugar de su IP pública o la de su pasarela NAT.

---

## 2. Conectividad y Multimedia

* **`ice_support=yes`**
  * **Qué hace:** Habilita el soporte para **ICE** (*Interactive Connectivity Establishment*).
  * **Para qué sirve:** Permite que Asterisk y el dispositivo negocien la mejor ruta posible para el tráfico de medios (RTP), evaluando conexiones directas, STUN o TURN. Es muy útil en entornos complejos con múltiples capas de NAT.

* **`direct_media=no`** *(antiguamente `canreinvite=no`)*
  * **Qué hace:** Evita que Asterisk intente que dos dispositivos hablen directamente entre sí (peer-to-peer) saltándose a Asterisk para el flujo de audio.
  * **Para qué sirve:** Al ponerlo en `no`, **todo el audio pasa obligatoriamente por Asterisk**. Esto es necesario si grabas llamadas, usas conferencias, realizas transferencias asistidas o si hay NAT estricto de por medio.

* **`disable_direct_media_on_nat=yes`**
  * **Qué hace:** Es una medida de seguridad complementaria. Si por alguna razón `direct_media` estuviera habilitado, esta opción lo desactiva automáticamente si detecta que alguna de las partes está detrás de un NAT.
  * **Para qué sirve:** Previene que llamadas fallen o queden en silencio cuando los teléfonos están tras firewalls corporativos o domésticos.

* **`use_avpf=no`**
  * **Qué hace:** Desactiva **AVPF** (*Audio-Visual Profile with Feedback*), que es la versión con retroalimentación del perfil RTP para WebRTC y video avanzado.
  * **Para qué sirve:** Al estar en `no`, se usa el perfil RTP tradicional (`RTP/AVP`), típico de telefonía SIP estándar (códecs como G.711 o G.729) donde no se requiere control de flujo avanzado por RTCP.

* **`inband_progress=no`**
  * **Qué hace:** Controla si Asterisk debe generar tonos de progreso de llamada (como el tono de marcado o de llamada en curso) dentro del flujo de audio (*in-band*) o si debe dejar que el teléfono los gestione mediante señalización SIP (*out-of-band*).
  * **Para qué sirve:** Al estar en `no`, Asterisk confía en los mensajes de señalización SIP estándar (`183 Session Progress`, etc.) para que sea el propio teléfono el que reproduzca los tonos al usuario.

---

## 3. Calidad de Servicio (QoS)

* **`tos_audio=ef`**
  * **Qué hace:** Define el valor **ToS** (*Type of Service*) o DSCP para los paquetes de **audio** salientes. `ef` significa **Expedited Forwarding** (Reenvío Expedito, valor binario `101110`).
  * **Para qué sirve:** Le indica a los switches y routers de la red que prioricen el tráfico de voz para evitar retardos o cortes (*jitter*).

* **`tos_video=af41`**
  * **Qué hace:** Define la marca ToS/DSCP para los paquetes de **video**. `af41` significa *Assured Forwarding Class 4, Drop Probability Low*.
  * **Para qué sirve:** Prioriza el tráfico de video en la red de manera adecuada frente a datos comunes, aunque inferior a la voz estricta.

* **`cos_audio=5`**
  * **Qué hace:** Establece la prioridad **CoS** (*Class of Service* / IEEE 802.1p) en la capa 2 (nivel de trama Ethernet/VLAN) para los paquetes de **audio**, con valor **5** (el estándar de la industria para voz).
  * **Para qué sirve:** Permite que los switches administrables prioricen el tráfico de voz a nivel de hardware antes de que salga al router.

* **`cos_video=4`**
  * **Qué hace:** Establece la prioridad **CoS** a nivel de capa 2 para los paquetes de **video**, con valor **4**.
  * **Para qué sirve:** Prioriza los tramos de red de video en conmutadores compatibles con VLANs y QoS de capa 2.

---

## 4. Suscripciones y Seguridad

* **`allow_subscribe=no`**
  * **Qué hace:** Bloquea o deshabilita las peticiones de suscripción SIP (`SUBSCRIBE`) procedentes de este endpoint.
  * **Para qué sirve:** Impide que el dispositivo solicite notificaciones de estado (por ejemplo, monitoreo de estado de extensiones mediante BLF - *Busy Lamp Field* o eventos de buzón de voz), útil si es una troncal con un proveedor o un dispositivo que no necesita esa funcionalidad.

* **`media_encryption=no`**
  * **Qué hace:** Desactiva el cifrado de medios (como **SRTP**).
  * **Para qué sirve:** El audio y el video viajan sin cifrado (en texto plano/RTP normal). Si estuviera en `yes` o `sdes`, se requeriría cifrar el flujo multimedia.