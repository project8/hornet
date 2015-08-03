Hornet
======

Hornet is the automatic data-flow software package for the Project 8 experiment.  It is responsible for handling data files after they are created by some DAQ system, including:

* Moving files to safer storage locations,

* Sending information about the files to the run database, and 

* Performing nearline processing on some or all of the files.

This documentation covers topics that are of interest to both users and developers.  Topics that are not necessary for running Hornet are marked with **(dev)**.

As you read through the Hornet documentation, you'll probably want to be aware of these :doc:`Concepts <concepts>`.  The Organization section below will take you through the individual modules that make up Hornet.


Basic Operation
---------------

For obtaining and building hornet, please see the README file.

The hornet executable is, appropriately, called ``hornet``.  It is configured with a JSON configuration file that is supplied at runtime::

  > hornet --config my_config.json

You can find an example configuration file that includes settings for all of the available options in the ``examples`` directory.

The links in the Organization section will provide details on how to configure each module.  File hashing is performed in multiple places, and has its own configuration as well.



Organization
------------

The various functions of Hornet are performed by a number of semi-independent modules:

* :doc:`Scheduler <modules/scheduler>` -- coordinate the processing of the files with the hornet modules.

* :doc:`Watcher <modules/watcher>` -- monitors a portion of the filesystem for new files to process;

* :doc:`Classifier <modules/classifier>` -- recognizes files of certain types and how to proces them;

* :doc:`Mover <modules/mover>` -- copies files from the "hot" data storage to the "warm" data storage;

* :doc:`Workers <modules/workers>` -- performs nearline analysis on some or all files;

* :doc:`Shipper <modules/shipper>` -- move files to a "cold" storage file system;

Other components: :doc:`AMQP <modules/amqp>`, and :doc:`Authentication <authentication>`

Repository
----------
**(dev)**

The directory structure of the ``hornet`` repository is as follows:

* The README, license, authors, source for the ``hornet`` executable, and the makefile for processing the git repository information exist in the top-level directory;
* ``documentation`` includes the documentation you're currently reading;
* ``examples`` includes the full example configuration file;
* ``gogitver`` contains the source for the external package ``gogitver``, which is used make hornet aware of the git repository information;
* ``hornet`` contains the source files for the ``hornet`` library, including all of the modules.


Logging
-------

Terminal output is classified into six levels:

1. Critical -- A fatal error has occurred. Hornet will shut down.
2. Error -- A non-fatal error has occurred. Hornet will continue, but the issue should be addressed.
3. Warn -- Something unexpected happened. Hornet will continue, but there may be an issue that needs to be addressed.
4. Notice -- Hornet is running just fine, and you should be aware of what's happening.
5. Info -- Hornet is running just fine, and here is some more verbose information about what's happening.
6. Debug -- Here are some verbose details about what's happening.

