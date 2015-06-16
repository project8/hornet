Classifier
==========

The Classifier is primarily responsible for identifying files according to a particular set of file types that it recognizes.  Unrecognized files are ignored.

It also has a number of other responsibilities:

* Determine the subdirectory path for each file (see the Directory Structure section of :doc:`Concepts <concepts>`);
* Calculating the initial hash of each file (see :doc:`Hashing <hash>`), and
* Sending file information via AMQP (see below).


Configuration
-------------

::

    "classifier":
    {
        "types":
        [
            {
                "name": "egg",
                "match-regexp": "rid(?P<run_id>[0-9]*)-([A-Za-z0-9_]*).egg",
                "do-hash": true
            },
            {
                "name": "rsa-mat",
                "match-regexp": "rid(?P<run_id>[0-9]*)-([A-Za-z0-9_]*).mat",
                "do-hash": true 
            },
            {
                "name": "rsa-setup",
                "match-extension": "Setup",
                "do-hash": false
            }
        ]
        "base-paths":
        [
            "/some/special/path",
            "/another/path"
        ],
        "send-file-info": true,
        "send-to": "hornet.print-message",
        "wait-for-sender": 2,
        "max-jobs": 25
    }

* ``types`` (array): the file types that can be recognized.
* ``[type].name`` (string): a unique identifier for the particular file type
* ``[type].match-regexp`` (string): one of the two options for file identification; a regular expression that will match the entire filename (not including directory path) according to the `regular expression syntax <http://golang.org/pkg/regexp/syntax>`_ in the Go standard library.
* ``[type].match-extension]`` (string): the second of the two options for file identification; a simple file-extension match that looks for the postfix of the filename after the last ``'.'``.
* ``[type].do-hash`` (boolean): whether or not to perform a hash that will be used to verify that the file is moved without any changes.
* ``base-paths`` (array of strings): paths that should be included in the list of base directories (see the Directory Structure section of :doc:`Concepts <concepts>`).
* ``send-file-info`` (boolean): whether or not to transmit the file information via AMQP.
* ``send-to`` (string): the AMQP routing key used to direct the file-information message.
* ``wait-for-sender`` (unsigned int): a configurable delay used to wait for the AMQP sender to be ready before starting the Classifier (since network delays can sometimes make AMQP initialization slow).
* ``max-jobs`` (unsigned int): the maximum number of jobs that can be assigned to any single file type.


Regular Expression Matching
---------------------------

This matching option uses the `regular expression syntax <http://golang.org/pkg/regexp/syntax>`_.

The use of subexpressions (e.g. ``([A-Za-z0-9_]*)``) is encouraged as a way to reliably identify filenames that have a standardized structure.

Additionally, named subexpressions (e.g. ``(?P<run_id>[0-9]*)``) are used in a special way.  Any named subexpression is automatically included in the file information that is sent via AMQP (see below; in the future this may be changed to be customizable).  Future developments may include the ability to use named subexpressions as variables for customizing commands (see :doc:`Workers <workers>`).


Sending File Information
------------------------

The Classifier can optionally send file information via AMQP.  This is particularly useful for filling in the database with information about each file.  In addition to the filename and hash, any named subexpressions from a regular expression match will be sent.  In the future this may be upgraded to be a configurable set of those subexpressions.
