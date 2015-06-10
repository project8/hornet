AMQP
====

Communication between Hornet and the other pieces of data-acquisition software is conducted via the AMQP 0.9.1 protocol.  In Hornet, this is used for the following purposes:

* Receiving remote commands (e.g. to quit);
* Sending file information to the database

Configuration
-------------

::

    "amqp":
    {
        "active": true,
        "broker": "my.server",
        "exchange": "requests",
        "queue": "hornet"
    }

* ``active`` (boolean): Determines whether or not AMQP communication is used.  If false, commands can not be received, and file information will not be sent.
* ``broker`` (string): The address of the AMQP broker that should be used.
* ``exchange`` (string): The name of the exchange that the receiver should listen to (and create, if it doesn't already exist).
* ``queue`` (string): The name of the queue that will be used to receive messages.  Any messages sent with this queue name as the first element of the routing key will be received by Hornet.  To specify the destination within hornet, further elements of the routing key should be used (see below).


Sending to Hornet
-----------------

Hornet's AMQP receiver will create a queue with the name specified in the configuration.  Hornet is actually listening for messages with any routing key that starts with that name (**(dev)** it is subscribed to ``[queue name].#``).  The following routing keys are available:

* ``[queue name].print-message``: prints the incoming message to the terminal
* ``[queue name].quit-mantis``: causes mantis to cleanly shutdown
