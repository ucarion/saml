# Example TodoApp with SAML

> Yes, this demo app is very ugly. It's not meant to be an example of modern
> UI/UX best practices. The hope is that by paring down the application to its
> absolute bare essentials, the result is something that's easy to digest.

This is an example of how you can use `github.com/ucarion/saml` to implement
SAML in a real-world application. You can try this example by running, from the
directory where this README is in:

```bash
docker-compose up -d
```

And then running:

```bash
go run ./...
```

You can now visit localhost:8080 and try out the example app.

## Guided Tour

Here's a suggested approach you can take to trying out this example. In this
guided tour, you'll see an example of an application supporting SAML, and
experience what setting up and logging in with SAML is like as an end user.

> If you're in a hurry, you can just do steps (1) and (2) and then jump to step
> (9) to jump straight to the SAML stuff. We include these other steps to
> emphasize that it's possible to have username/password and SAML coexist
> without any problems.

1. First, you'll need to sign up for a free account with [Okta][okta] or
   [OneLogin][onelogin], which are widely-used Identity Providers that support
   free trials.

1. Next, create an account in the demo app. Visit localhost:8080, and put in
   "test" and "password" in the Sign Up form.

1. You should now be logged in and see information about the SAML Configuration,
   Users, and Todos in your newly-created account. Try creating a todo, by
   typing something into the input at the bottom of the page and clicking
   "Create a todo".

1. You should now see your todo on the page. It should also have the word
   "test:" next to it -- that's just showing you that the user named "test"
   wrote that todo.

1. Before jumping into the SAML stuff, let's invite another user into our
   account. In the "Users" section, so far there's only one user, named "test".
   Let's create another. Put "bob" in the Display Name section and "password" in
   the password section, and click "Create a user".

1. Bob should now be in the list of users. Let's log in as them now. Copy-paste
   the long UUID next to their name -- it should look something like:

   ```text
   c9d87570-e17b-4621-8af1-4053b6320c5c
   ```

1. Go to localhost:8080 again, and paste the ID you just copied into the User ID
   under "Log in", and put "password" into the password field. Click "Log into
   an existing user".

1. At the top of the page, you should now see that it says that you're logged in
   as bob.

1. Now, let's try setting up SAML. First, we need to set up the connection from
   the Identity Provider end.

   Under the SAML Configuration section, you should see a table labeled "Data
   you need to put into your Identity Provider". Let's now go to our Identity
   Provider, and create a new connection with this information.

   * If you're using OneLogin, go to your OneLogin administrator page. Go to
     "Applications", and then click "Add App". Choose "SAML Test Connector
     (Advanced)" -- you can use the search to help you find this. Click "Save".

     You've now created a new OneLogin application. We can now paste the
     information from TodoApp into OneLogin. Go to "Configuration", and then put
     the TodoApp "SAML Recipient ID" into the OneLogin input labeled
     "Recipient". Put the TodoApp "SAML Assertion Consumer Service ("ACS") URL"
     into both "ACS (Consumer) URL Validator" and "ACS (Consumer) URL".

     Then hit save.

   * If you're using Okta, go to your Okta admin dashboard. Go to
     "Applications", and then click "Add Application". Choose "Create New App"
     at the top right, and then the "SAML 2.0" option. Then click "Create".

     Give the application any name you like, and then click Next.

     Copy the "SAML Assertion Consumer Service ("ACS") URL" from TodoApp into
     the field in Okta called "Single sign on URL". Untick the box labeled "Use
     this for Recipient URL and Destination URL". Copy "SAML Recipient ID" from
     TodoApp into "Audience URI (SP Entity ID)" and "Recipient URL" in Okta. The
     "Destination URL" should be the same thing as the "Single sign on URL" in
     Okta. Click "Next".

     Do whatever you like on the "Feedback" page. It doesn't matter. Click
     "Finish".

1. Next we need to set up SAML from the Service Provider end. In this case,
   TodoApp is the service provider.

   * If you're using OneLogin, click on the "More Actions" dropdown at the top
     right of the edit page for the OneLogin app you just created. Click on the
     "SAML Metadata" option with a download icon.

     Now, go back to localhost:8080, and click on the button to the left of the
     "Upload Identity Provider SAML Metadata" button. A file chooser will appear
     -- choose the XML file you just downloaded from OneLogin, and then click
     "Upload Identity Provider SAML Metadata".

   * If you're using Okta, click on the "Sign On" tab of your application. A
     yellow-bordered box contains a link labeled "Identity Provider metadata".
     Click on that link. Save the page. Your browser may ask what format you
     want to save it in -- choose "XML", not "Web page".

     Now, go back to localhost:8080, and click on the button to the left of the
     "Upload Identity Provider SAML Metadata" button. A file chooser will appear
     -- choose the XML file you just downloaded/copied from Okta, and then click
     "Upload Identity Provider SAML Metadata".

     You'll also need to assign the app to yourself at this time. Go to
     "Assignments", click "Assign" and then "Assign People". Find your Okta user
     in the list of users, click "Assign", and then "Save and Go Back", then
     "Done".

1. The table called "Data from your Identity Provider you need to give us"
   should now be populated with a bunch of data. These are pieces of information
   that TodoApp will need in order to verify the authenticity of the SAML logins
   it receives.

1. Try clicking on "Initiate SAML Login Flow". You should find yourself briefly
   being redirected to your identity provider (this might happen so fast you
   can't see it -- check your browser's network logs for proof it's really
   happening), and then being redirected back.

1. At the top of the page, it should now say you're logged in as
   "xxx@yourcompany.com". That's because when TodoApp got the SAML login request
   from your Identity Provider, it auto-created an account for you and then
   logged you in as them.

   You can see that "xxx@yourcompany.com" email appears under the "Users"
   section.

1. As a last little test, you can try logging into TodoApp directly from your
   Identity Provider, instead of relying on the "Initiate SAML Login Flow" link.
   Find the app you created in step (9), and clicking on it. You may need to
   exit the "Admin" view of your Identity Provider, so that it logs you in
   instead of trying to edit the connection details.

   When you log in from your Identity Provider like this, it's called
   "Identity-Provider (IdP) Initiated Login". When we logged in by clicking on
   the link in TodoApp, that was "Service-Provider (SP) Initiated Login".

[okta]: https://www.okta.com/free-trial/
[onelogin]: https://www.onelogin.com/free-trial
